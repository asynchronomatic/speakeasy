package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/config"
)

func TestConfigPath(t *testing.T) {
	dir := t.TempDir()
	config.SetConfigPath(dir)

	err := config.NewManager(config.DefaultConfigPath).InitializeFromDefaults()
	require.NoError(t, err)

	writeTestFile(t, config.DefaultNodePath, []byte("node-identity"))
	writeTestFile(t, config.DefaultRelayPath, []byte("node-identity"))

	_, err = os.Stat(path.Join(dir, "config.yaml"))
	assert.NoError(t, err)

	_, err = os.Stat(path.Join(dir, "node.key"))
	assert.NoError(t, err)

	_, err = os.Stat(path.Join(dir, "relay.key"))
	assert.NoError(t, err)
}

func TestRunJoin(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	cm := config.NewManager(config.DefaultConfigPath)
	err := cm.InitializeFromDefaults()
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		var req api.RedeemInviteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if req.Node.ID == "" {
			t.Error("missing node id")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.RedeemInviteResponse{
			MeshId:     "default",
			MeshSecret: "join-secret",
			MeshServer: "http://10.0.0.30:4002",
		})
	}))
	t.Cleanup(ts.Close)

	askProxyPassword = func() (string, error) { return "join-pass", nil }
	t.Cleanup(func() { askProxyPassword = promptProxyPassword })

	if err := joinWithInvite(ts.URL + "/api/v1/redeem/abc"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(config.DefaultConfigPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.DefaultNodePath); err != nil {
		t.Fatal(err)
	}

	cm = config.NewManager(config.DefaultConfigPath)
	err = cm.ReadConfig(func(cfg *config.Config) error {
		if cfg.Mesh.Address != "http://10.0.0.30:4002" || cfg.Mesh.Secret != "join-secret" || cfg.Mesh.MeshId != "default" {
			t.Fatalf("loaded mesh %+v", cfg.Mesh)
		}
		if cfg.Proxy.Password != "join-pass" {
			t.Fatalf("proxy password %q", cfg.Proxy.Password)
		}
		return nil
	})
	assert.NoError(t, err)

}

func TestRunJoinExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	cm := config.NewManager(config.DefaultConfigPath)
	err := cm.InitializeFromDefaults()
	require.NoError(t, err)

	cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.Listen = ":7777"
		cfg.Proxy.Password = "keep-pass"
		cfg.Admin.Secret = "keep-admin"
		cfg.Mesh.Name = "box-1"
		cfg.Mesh.Address = "http://old:4002"
		cfg.Mesh.Secret = "old-secret"
		cfg.Mesh.MeshId = "old-mesh"
		cfg.Mesh.MDNSEnabled = false
		cfg.Providers = []config.Provider{{
			ID:      "custom",
			Type:    "openai",
			BaseURL: "http://127.0.0.1:8080",
		}}
		return nil
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.RedeemInviteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if req.Node.Name != "box-1" {
			t.Errorf("node name %q", req.Node.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.RedeemInviteResponse{
			MeshId:     "default",
			MeshSecret: "join-secret",
			MeshServer: "http://10.0.0.30:4002",
		})
	}))
	t.Cleanup(ts.Close)

	askProxyPassword = func() (string, error) {
		t.Fatal("should not prompt when proxy.password is set")
		return "", nil
	}
	t.Cleanup(func() { askProxyPassword = promptProxyPassword })

	err = joinWithInvite(ts.URL + "/api/v1/redeem/abc")
	require.NoError(t, err)

	cm = config.NewManager(config.DefaultConfigPath)
	err = cm.ReadConfig(func(cfg *config.Config) error {
		if cfg.Mesh.Address != "http://10.0.0.30:4002" || cfg.Mesh.Secret != "join-secret" || cfg.Mesh.MeshId != "default" {
			t.Fatalf("updated mesh %+v", cfg.Mesh)
		}
		if cfg.Proxy.Listen != ":7777" || cfg.Proxy.Password != "keep-pass" || cfg.Admin.Secret != "keep-admin" || cfg.Mesh.Name != "box-1" || cfg.Mesh.MDNSEnabled {
			t.Fatalf("kept settings %+v", cfg)
		}
		if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "custom" {
			t.Fatalf("providers %+v", cfg.Providers)
		}
		return nil
	})
	assert.NoError(t, err)
}

func TestJoinExistingWithoutPasswordPrompts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	cm := config.NewManager(config.DefaultConfigPath)
	err := cm.InitializeFromDefaults()
	require.NoError(t, err)

	err = cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.Listen = ":7777"
		cfg.Mesh.Name = "box-1"
		return nil
	})
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.RedeemInviteResponse{
			MeshId:     "default",
			MeshSecret: "join-secret",
			MeshServer: "http://10.0.0.30:4002",
		})
	}))
	t.Cleanup(ts.Close)

	asked := false
	askProxyPassword = func() (string, error) {
		asked = true
		return "new-pass", nil
	}
	t.Cleanup(func() { askProxyPassword = promptProxyPassword })

	if err := joinWithInvite(ts.URL + "/api/v1/redeem/abc"); err != nil {
		t.Fatal(err)
	}
	if !asked {
		t.Fatal("expected password prompt")
	}

	cm = config.NewManager(config.DefaultConfigPath)
	err = cm.EnsureLoaded()
	require.NoError(t, err)

	err = cm.ReadConfig(func(cfg *config.Config) error {
		assert.Equal(t, "new-pass", cfg.Proxy.Password)
		return nil
	})
	assert.NoError(t, err)

}

func TestEnsureProxyPasswordSkipsWhenSet(t *testing.T) {
	cfg := &config.Config{}
	cfg.Proxy.Password = "already"
	askProxyPassword = func() (string, error) {
		t.Fatal("should not prompt")
		return "", nil
	}
	t.Cleanup(func() { askProxyPassword = promptProxyPassword })
	if err := ensureProxyPassword(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Password != "already" {
		t.Fatalf("password %q", cfg.Proxy.Password)
	}
}

func TestExistingJoinConfigAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

}

func TestExistingJoinWarning(t *testing.T) {
	cfg := &config.Config{}
	cfg.Mesh.MeshId = "default"
	cfg.Mesh.Address = "http://10.0.0.30:4002"
	got := existingJoinWarning("/tmp/mesh/config.yaml", cfg)
	for _, want := range []string{
		"/tmp/mesh/config.yaml",
		"Current mesh: default @ http://10.0.0.30:4002",
		"wrong directory",
		"cannot be undone",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestAdminControllerAddr(t *testing.T) {
	cfg := &config.Config{}
	if _, _, ok := adminControllerAddr(cfg); ok {
		t.Fatal("empty config should not attach admin")
	}
	cfg.Admin.Secret = "sekrit"
	if _, _, ok := adminControllerAddr(cfg); ok {
		t.Fatal("secret without address should not attach")
	}
	cfg.Admin.Address = "http://127.0.0.1:4002"
	addr, secret, ok := adminControllerAddr(cfg)
	if !ok || addr != "http://127.0.0.1:4002" || secret != "sekrit" {
		t.Fatalf("got %q %q %v", addr, secret, ok)
	}
	cfg.Admin.Address = ""
	cfg.Mesh.Address = "http://10.0.0.30:4002"
	addr, _, ok = adminControllerAddr(cfg)
	if !ok || addr != "http://10.0.0.30:4002" {
		t.Fatalf("mesh fallback %q %v", addr, ok)
	}
}

func TestRunJoinRequiresURL(t *testing.T) {
	if err := runJoin(""); err == nil {
		t.Fatal("expected usage error")
	}
}
