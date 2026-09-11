package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/asynchronomatic/speakeasy/pkg/config"
)

func TestResetMembership(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cfg := &config.Config{}
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
	writeTestConfig(t, cfg)
	writeTestFile(t, defaultNodeKeyPath, []byte("node-identity"))
	writeTestFile(t, defaultRelayKeyPath, []byte("relay-identity"))

	confirmReset = func(title, description string) (bool, error) {
		if title == "" || description == "" {
			t.Fatal("expected confirmation title and description")
		}
		if !containsAll(description, "admin.address", "admin.secret", "mesh.address", "mesh.mesh_id", defaultNodeKeyPath) {
			t.Fatalf("description missing membership details:\n%s", description)
		}
		if strings.Contains(description, "admin-secret") {
			t.Fatalf("description leaked admin secret:\n%s", description)
		}
		return true, nil
	}
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil {
		t.Fatal(err)
	}

	got, err := config.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if got.Admin.Address != "" || got.Admin.Secret != "" {
		t.Fatalf("admin membership still set %+v", got.Admin)
	}
	if got.Mesh.Address != "" || got.Mesh.MeshId != "" {
		t.Fatalf("mesh membership still set %+v", got.Mesh)
	}
	if got.Proxy.Listen != ":7777" || got.Proxy.Password != "keep-pass" || got.Admin.AdminPort != 4111 {
		t.Fatalf("kept settings lost proxy=%+v admin=%+v", got.Proxy, got.Admin)
	}
	if got.Mesh.Name != "box-1" || !got.Mesh.MDNSEnabled || got.Mesh.Secret != "mesh-secret" {
		t.Fatalf("kept mesh extras %+v", got.Mesh)
	}
	if len(got.Providers) != 1 || got.Providers[0].ID != "custom" {
		t.Fatalf("providers %+v", got.Providers)
	}
	if fileExists(defaultNodeKeyPath) {
		t.Fatal("expected node.key removed")
	}
	if !fileExists(defaultRelayKeyPath) {
		t.Fatal("relay.key should be kept")
	}
}

func TestResetMembershipAborted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cfg := &config.Config{}
	cfg.Admin.Secret = "admin-secret"
	cfg.Mesh.MeshId = "default"
	writeTestConfig(t, cfg)
	writeTestFile(t, defaultNodeKeyPath, []byte("node-identity"))

	confirmReset = func(string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if got.Admin.Secret != "admin-secret" || got.Mesh.MeshId != "default" {
		t.Fatalf("config changed on abort %+v", got)
	}
	if !fileExists(defaultNodeKeyPath) {
		t.Fatal("node.key removed on abort")
	}
}

func TestResetAll(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTestConfig(t, &config.Config{})
	writeTestFile(t, defaultNodeKeyPath, []byte("node-identity"))
	writeTestFile(t, defaultRelayKeyPath, []byte("relay-identity"))

	confirmReset = func(title, description string) (bool, error) {
		if !containsAll(description, defaultConfigPath, defaultNodeKeyPath) {
			t.Fatalf("description missing files:\n%s", description)
		}
		return true, nil
	}
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(true); err != nil {
		t.Fatal(err)
	}
	if fileExists(defaultConfigPath) {
		t.Fatal("expected config.yaml removed")
	}
	if fileExists(defaultNodeKeyPath) {
		t.Fatal("expected node.key removed")
	}
	if !fileExists(defaultRelayKeyPath) {
		t.Fatal("relay.key should be kept")
	}
}

func TestResetCLIAll(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTestConfig(t, &config.Config{})
	writeTestFile(t, defaultNodeKeyPath, []byte("node-identity"))

	confirmReset = func(string, string) (bool, error) { return true, nil }
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	cmd := newCommand()
	cmd.Writer = io.Discard
	if err := cmd.Run(context.Background(), []string{"mesh", "reset", "--all"}); err != nil {
		t.Fatal(err)
	}
	if fileExists(defaultConfigPath) || fileExists(defaultNodeKeyPath) {
		t.Fatal("expected config.yaml and node.key removed")
	}
}

func TestResetNodeKeyOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTestFile(t, defaultNodeKeyPath, []byte("node-identity"))

	confirmReset = func(string, string) (bool, error) { return true, nil }
	t.Cleanup(func() { confirmReset = promptResetConfirm })

	if err := runReset(false); err != nil {
		t.Fatal(err)
	}
	if fileExists(defaultNodeKeyPath) {
		t.Fatal("expected node.key removed")
	}
}

func TestResetNothing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

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

func writeTestConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, defaultConfigPath, data)
}

func writeTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(".", name), data, 0o600); err != nil {
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
