package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
)

func TestConfigFromInvite(t *testing.T) {
	cfg := configFromInvite(&api.RedeemInviteResponse{
		MeshId:     "default",
		MeshSecret: "sekrit",
		MeshServer: "http://10.0.0.30:4002",
	}, nil)
	if cfg.Mesh.Address != "http://10.0.0.30:4002" {
		t.Fatalf("Address=%q", cfg.Mesh.Address)
	}
	if cfg.Mesh.MeshId != "default" || cfg.Mesh.Secret != "sekrit" {
		t.Fatalf("mesh %+v", cfg.Mesh)
	}
	if cfg.Proxy.Listen != core.DefaultProxyListen {
		t.Fatalf("listen %q", cfg.Proxy.Listen)
	}
	if !cfg.Proxy.AllowPrivateBackends {
		t.Fatal("join default should allow private backends for local Ollama")
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Type != "ollama" {
		t.Fatalf("providers %+v", cfg.Providers)
	}
}

func TestConfigFromInviteKeepsExisting(t *testing.T) {
	existing := &core.Config{}
	existing.Proxy.Listen = ":9999"
	existing.Proxy.Password = "kept-pass"
	existing.Admin.AdminPort = 4111
	existing.Admin.Secret = "admin-secret"
	existing.Mesh.Name = "kept-name"
	existing.Mesh.Address = "http://old:4002"
	existing.Mesh.Secret = "old-secret"
	existing.Mesh.MeshId = "old-mesh"
	existing.Mesh.MDNSEnabled = false
	existing.Mesh.ForcePrivate = true
	existing.Mesh.Port = 1234
	existing.Providers = []core.Provider{{
		ID:      "custom",
		Type:    "openai",
		BaseURL: "http://127.0.0.1:8080",
	}}

	cfg := configFromInvite(&api.RedeemInviteResponse{
		MeshId:     "default",
		MeshSecret: "new-secret",
		MeshServer: "http://10.0.0.30:4002",
	}, existing)
	if cfg.Mesh.Address != "http://10.0.0.30:4002" || cfg.Mesh.Secret != "new-secret" || cfg.Mesh.MeshId != "default" {
		t.Fatalf("updated mesh %+v", cfg.Mesh)
	}
	if cfg.Proxy.Listen != ":9999" || cfg.Proxy.Password != "kept-pass" || cfg.Admin.AdminPort != 4111 || cfg.Admin.Secret != "admin-secret" {
		t.Fatalf("kept settings proxy=%+v admin=%+v", cfg.Proxy, cfg.Admin)
	}
	if cfg.Mesh.Name != "kept-name" || cfg.Mesh.MDNSEnabled || !cfg.Mesh.ForcePrivate || cfg.Mesh.Port != 1234 {
		t.Fatalf("kept mesh extras %+v", cfg.Mesh)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "custom" {
		t.Fatalf("providers %+v", cfg.Providers)
	}
}

func TestRunJoin(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

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

	if err := joinWithInvite(ts.URL+"/api/v1/redeem/abc", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, defaultConfigPath)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultNodeKeyPath)); err != nil {
		t.Fatal(err)
	}

	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mesh.Address != "http://10.0.0.30:4002" || cfg.Mesh.Secret != "join-secret" || cfg.Mesh.MeshId != "default" {
		t.Fatalf("loaded mesh %+v", cfg.Mesh)
	}
	if cfg.Proxy.Password != "join-pass" {
		t.Fatalf("proxy password %q", cfg.Proxy.Password)
	}
}

func TestRunJoinExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	existing := &core.Config{}
	existing.Proxy.Listen = ":7777"
	existing.Proxy.Password = "keep-pass"
	existing.Admin.Secret = "keep-admin"
	existing.Mesh.Name = "box-1"
	existing.Mesh.Address = "http://old:4002"
	existing.Mesh.Secret = "old-secret"
	existing.Mesh.MeshId = "old-mesh"
	existing.Mesh.MDNSEnabled = false
	existing.Providers = []core.Provider{{
		ID:      "custom",
		Type:    "openai",
		BaseURL: "http://127.0.0.1:8080",
	}}
	data, err := yaml.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultConfigPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

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

	if err := joinWithInvite(ts.URL+"/api/v1/redeem/abc", existing); err != nil {
		t.Fatal(err)
	}

	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mesh.Address != "http://10.0.0.30:4002" || cfg.Mesh.Secret != "join-secret" || cfg.Mesh.MeshId != "default" {
		t.Fatalf("updated mesh %+v", cfg.Mesh)
	}
	if cfg.Proxy.Listen != ":7777" || cfg.Proxy.Password != "keep-pass" || cfg.Admin.Secret != "keep-admin" || cfg.Mesh.Name != "box-1" || cfg.Mesh.MDNSEnabled {
		t.Fatalf("kept settings %+v", cfg)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "custom" {
		t.Fatalf("providers %+v", cfg.Providers)
	}
}

func TestJoinExistingWithoutPasswordPrompts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	existing := &core.Config{}
	existing.Proxy.Listen = ":7777"
	existing.Mesh.Name = "box-1"

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

	if err := joinWithInvite(ts.URL+"/api/v1/redeem/abc", existing); err != nil {
		t.Fatal(err)
	}
	if !asked {
		t.Fatal("expected password prompt")
	}
	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Password != "new-pass" {
		t.Fatalf("password %q", cfg.Proxy.Password)
	}
}

func TestEnsureProxyPasswordSkipsWhenSet(t *testing.T) {
	cfg := &core.Config{}
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
	t.Chdir(t.TempDir())
	cfg, cont, err := existingJoinConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cont || cfg != nil {
		t.Fatalf("cont=%v cfg=%v", cont, cfg)
	}
}

func TestExistingJoinWarning(t *testing.T) {
	cfg := &core.Config{}
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
	cfg := &core.Config{}
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
