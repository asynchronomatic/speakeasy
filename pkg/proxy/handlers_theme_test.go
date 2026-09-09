package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

func TestThemeGetDefaultsDeco(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, core.DefaultTheme, resp.Theme)
}

func TestThemeGetStored(t *testing.T) {
	writeTestConfig(t, `proxy:
  listen: ":4080"
  theme: clean
mesh:
  address: http://x
`)
	p := testProxy(t)
	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "clean", resp.Theme)
}

func TestThemeGetInvalidDefaultsDeco(t *testing.T) {
	writeTestConfig(t, `proxy:
  listen: ":4080"
  theme: nope
mesh:
  address: http://x
`)
	p := testProxy(t)
	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, core.DefaultTheme, resp.Theme)
}

func TestThemeSetPersists(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", &themeResponse{Theme: "Clean"}, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "clean", resp.Theme)

	cfg, err := core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Theme != "clean" {
		t.Fatalf("saved theme %q", cfg.Proxy.Theme)
	}
	if cfg.Proxy.Listen != ":4080" || len(cfg.Providers) != 1 || cfg.Providers[0].ID != "local" {
		t.Fatalf("other fields changed: %+v", cfg)
	}

	err = doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "clean", resp.Theme)
}

func TestThemeSetEmptyDefaultsDeco(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	resp := themeResponse{}

	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", &themeResponse{Theme: ""}, &resp)
	assert.NoError(t, err)
	assert.Equal(t, core.DefaultTheme, resp.Theme)

	cfg, err := core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Theme != core.DefaultTheme {
		t.Fatalf("saved theme %q", cfg.Proxy.Theme)
	}
}

func TestThemeSetInvalid(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", themeResponse{Theme: "neon"}, nil)
	assert.Error(t, err)

	cfg, err := core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Theme != "" {
		t.Fatalf("invalid set wrote %q", cfg.Proxy.Theme)
	}
}
