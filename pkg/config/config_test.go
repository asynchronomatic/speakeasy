package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadSaveConfigProviders(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	SetConfigPath(dir)

	src := []byte(`proxy:
  listen: ":9"
admin:
  secret: s
mesh:
  name: n1
  address: http://example
providers:
- id: local
  type: ollama
  base_url: http://127.0.0.1:11434
  private: false
  model_discovery: pinned
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), src, 0o600); err != nil {
		t.Fatal(err)
	}

	cm := NewManager(DefaultConfigPath)

	err := cm.EnsureLoaded()
	require.NoError(t, err)

	err = cm.UpdateConfig(func(cfg *Config) error {
		if cfg.Proxy.Listen != ":9" || cfg.Mesh.Name != "n1" {
			t.Fatalf("loaded %+v", cfg)
		}
		if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "local" || cfg.Providers[0].BaseURL != "http://127.0.0.1:11434" {
			t.Fatalf("providers %+v", cfg.Providers)
		}

		cfg.Proxy.AllowPrivateBackends = true
		cfg.Providers = append(cfg.Providers, Provider{
			ID:        "cloud",
			Type:      "openai",
			BaseURL:   "https://api.example",
			Token:     "tok",
			Private:   true,
			Discovery: "whitelist",
		})
		return nil
	})
	require.NoError(t, err)

	// full reload
	cm = NewManager(DefaultConfigPath)
	err = cm.EnsureLoaded()
	require.NoError(t, err)
	cm.ReadConfig(func(cfg *Config) error {
		if cfg.Proxy.Listen != ":9" || cfg.Admin.Secret != "s" || cfg.Mesh.Address != "http://example" {
			t.Fatalf("other fields changed: %+v", cfg)
		}
		if !cfg.Proxy.AllowPrivateBackends {
			t.Fatal("allow_private_backends not saved")
		}
		if len(cfg.Providers) != 2 || cfg.Providers[1].ID != "cloud" || cfg.Providers[1].Token != "tok" {
			t.Fatalf("saved providers %+v", cfg.Providers)
		}
		return nil
	})
	require.NoError(t, err)

}

func TestNormalizeTheme(t *testing.T) {
	cases := map[string]string{
		"":      DefaultTheme,
		"  ":    DefaultTheme,
		"nope":  DefaultTheme,
		"Deco":  "deco",
		"clean": "clean",
		"NIGHT": "night",
		"cyber": "cyber",
	}
	for in, want := range cases {
		if got := NormalizeTheme(in); got != want {
			t.Fatalf("NormalizeTheme(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLoadConfigDefaultsTheme(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	SetConfigPath(dir)

	src := []byte(`proxy:
  listen: ":9"
mesh:
  address: http://example
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), src, 0o600); err != nil {
		t.Fatal(err)
	}

	cm := NewManager(DefaultConfigPath)
	err := cm.UpdateConfig(func(cfg *Config) error {
		if cfg.Proxy.Theme != DefaultTheme {
			t.Fatalf("default theme %q want %q", cfg.Proxy.Theme, DefaultTheme)
		}
		cfg.Proxy.Theme = "clean"
		return nil
	})
	require.NoError(t, err)

	cm = NewManager(DefaultConfigPath)
	err = cm.ReadConfig(func(cfg *Config) error {
		if cfg.Proxy.Theme != "clean" || cfg.Proxy.Listen != ":9" {
			t.Fatalf("saved theme %+v", cfg.Proxy)
		}
		return nil
	})
	require.NoError(t, err)
}
