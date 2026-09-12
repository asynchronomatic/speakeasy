package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/pkg/config"
)

func TestResetMembership(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	cm := config.NewManager(config.DefaultConfigPath)
	err := cm.InitializeFromDefaults()
	require.NoError(t, err)

	err = cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.Listen = ":7777"
		cfg.Proxy.Password = "keep-pass"
		cfg.Admin.Address = "http://10.0.0.30:4002"
		cfg.Admin.Secret = "admin-secret"
		cfg.Admin.AdminPort = 4111
		cfg.Mesh.Name = "box-1"
		cfg.Mesh.Address = "http://10.0.0.30:4002"
		cfg.Mesh.Secret = "mesh-secret"
		cfg.Mesh.MeshId = "default"
		cfg.Mesh.MDNSEnabled = true
		cfg.Providers = []config.Provider{{
			ID:      "custom",
			Type:    "openai",
			BaseURL: "http://127.0.0.1:8080",
		}}
		return nil
	})
	require.NoError(t, err)

	writeTestFile(t, config.DefaultNodePath, []byte("node-identity"))
	writeTestFile(t, config.DefaultRelayPath, []byte("relay-identity"))

	confirmReset = func(title, description string) (bool, error) {
		assert.NotEqual(t, "", title)
		assert.NotEqual(t, "", description)

		if title == "" || description == "" {
			t.Fatal("expected confirmation title and description")
		}

		all := containsAll(description, "admin.address", "admin.secret", "mesh.address", "mesh.mesh_id", config.DefaultNodePath)
		assert.Equalf(t, true, all, fmt.Sprintf("description missing membership details:\n%s", description))
		assert.NotContains(t, description, "admin-secret")
		return true, nil
	}
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil {
		t.Fatal(err)
	}

	cm = config.NewManager(config.DefaultConfigPath)
	err = cm.ReadConfig(func(cfg *config.Config) error {
		assert.Equal(t, "", cfg.Admin.Secret)
		assert.Equal(t, "", cfg.Admin.Address)
		assert.Equal(t, "", cfg.Mesh.MeshId)
		assert.Equal(t, "", cfg.Mesh.Address)
		assert.Equal(t, ":7777", cfg.Proxy.Listen)
		assert.Equal(t, "keep-pass", cfg.Proxy.Password)
		assert.Equal(t, 4111, cfg.Admin.AdminPort)
		assert.Equal(t, "box-1", cfg.Mesh.Name)
		assert.Equal(t, "mesh-secret", cfg.Mesh.Secret)
		assert.Equal(t, true, cfg.Mesh.MDNSEnabled)
		assert.Equal(t, 1, len(cfg.Providers))
		assert.Equal(t, "custom", cfg.Providers[0].ID)
		return nil
	})
	require.NoError(t, err)

	if fileExists(config.DefaultNodePath) {
		t.Fatal("expected node.key removed")
	}
	if !fileExists(config.DefaultRelayPath) {
		t.Fatal("relay.key should be kept")
	}
}

func TestResetMembershipAborted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	cm := config.NewManager(config.DefaultConfigPath)
	err := cm.InitializeFromDefaults()
	require.NoError(t, err)

	err = cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Admin.Secret = "admin-secret"
		cfg.Mesh.MeshId = "default"
		return nil
	})
	require.NoError(t, err)

	writeTestFile(t, config.DefaultNodePath, []byte("node-identity"))

	confirmReset = func(string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil && err.Error() != "aborted" {
		t.Fatal(err)
	}

	cm = config.NewManager(config.DefaultConfigPath)
	err = cm.ReadConfig(func(cfg *config.Config) error {
		assert.Equal(t, cfg.Admin.Secret, "admin-secret")
		assert.Equal(t, cfg.Mesh.MeshId, "default")
		return nil
	})
	require.NoError(t, err)

	if !fileExists(config.DefaultNodePath) {
		t.Fatal("node.key removed on abort")
	}
}

func TestResetAll(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	cm := config.NewManager(config.DefaultConfigPath)
	err := cm.InitializeFromDefaults()
	require.NoError(t, err)

	writeTestFile(t, config.DefaultNodePath, []byte("node-identity"))
	writeTestFile(t, config.DefaultRelayPath, []byte("relay-identity"))

	confirmReset = func(title, description string) (bool, error) {
		if !containsAll(description, config.DefaultConfigPath, config.DefaultNodePath) {
			t.Fatalf("description missing files:\n%s", description)
		}
		return true, nil
	}
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(true); err != nil {
		t.Fatal(err)
	}
	if fileExists(config.DefaultConfigPath) {
		t.Fatal("expected config.yaml removed")
	}
	if fileExists(config.DefaultNodePath) {
		t.Fatal("expected node.key removed")
	}
	if fileExists(config.DefaultRelayPath) {
		t.Fatal("relay.key should be kept")
	}
}

func TestResetCLIAll(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	err := config.NewManager(config.DefaultConfigPath).InitializeFromDefaults()
	require.NoError(t, err)

	writeTestFile(t, config.DefaultNodePath, []byte("node-identity"))

	confirmReset = func(string, string) (bool, error) { return true, nil }
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	cmd := newCommand()
	cmd.Writer = io.Discard
	if err := cmd.Run(context.Background(), []string{"mesh", "reset", "--all"}); err != nil {
		t.Fatal(err)
	}
	if fileExists(config.DefaultConfigPath) || fileExists(config.DefaultNodePath) {
		t.Fatal("expected config.yaml and node.key removed")
	}
}

func TestResetNodeKeyOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)
	writeTestFile(t, config.DefaultNodePath, []byte("node-identity"))

	confirmReset = func(string, string) (bool, error) { return true, nil }
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil {
		t.Fatal(err)
	}
	if fileExists(config.DefaultNodePath) {
		t.Fatal("expected node.key removed")
	}
}

func TestResetNothing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	config.SetConfigPath(dir)

	confirmReset = func(string, string) (bool, error) {
		t.Fatal("should not prompt when there is nothing to reset")
		return true, nil
	}
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil {
		t.Fatal(err)
	}
	if err := runReset(true); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(s, n) {
			return false
		}
	}
	return true
}
