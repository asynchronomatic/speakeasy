package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveConfigProviders(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

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

	cfg, err := LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
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
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	again, err := LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if again.Proxy.Listen != ":9" || again.Admin.Secret != "s" || again.Mesh.Address != "http://example" {
		t.Fatalf("other fields changed: %+v", again)
	}
	if !again.Proxy.AllowPrivateBackends {
		t.Fatal("allow_private_backends not saved")
	}
	if len(again.Providers) != 2 || again.Providers[1].ID != "cloud" || again.Providers[1].Token != "tok" {
		t.Fatalf("saved providers %+v", again.Providers)
	}
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
	src := []byte(`proxy:
  listen: ":9"
mesh:
  address: http://example
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), src, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Theme != DefaultTheme {
		t.Fatalf("default theme %q want %q", cfg.Proxy.Theme, DefaultTheme)
	}

	cfg.Proxy.Theme = "clean"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	again, err := LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if again.Proxy.Theme != "clean" || again.Proxy.Listen != ":9" {
		t.Fatalf("saved theme %+v", again.Proxy)
	}
}
