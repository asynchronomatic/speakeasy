package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/testable"
)

func TestThemeGetDefaultsDeco(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, config.DefaultTheme, resp.Theme)
}

func TestThemeGetStored(t *testing.T) {
	cm := testable.MustConfigManager(`proxy:
  listen: ":4080"
  theme: clean
  password: ` + ProxyLoginHash + `
mesh:
  address: http://x
`)
	p := newTestProxy(t, cm)

	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "clean", resp.Theme)
}

func TestThemeGetInvalidDefaultsDeco(t *testing.T) {
	cm := testable.MustConfigManager(`proxy:
  listen: ":4080"
  theme: nope
  password: ` + ProxyLoginHash + `
mesh:
  address: http://x
`)
	p := newTestProxy(t, cm)
	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, config.DefaultTheme, resp.Theme)
}

func TestThemeSetPersists(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", &themeResponse{Theme: "Clean"}, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "clean", resp.Theme)

	cfg := cm.Config()
	assert.Equal(t, "clean", cfg.Proxy.Theme)

	err = doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "clean", resp.Theme)
}

func TestThemeSetEmptyDefaultsDeco(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	resp := themeResponse{}
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", &themeResponse{Theme: ""}, &resp)
	assert.NoError(t, err)
	assert.Equal(t, config.DefaultTheme, resp.Theme)

	cfg := cm.Config()
	assert.Equal(t, config.DefaultTheme, cfg.Proxy.Theme)
}

func TestThemeSetInvalid(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", themeResponse{Theme: "neon"}, nil)
	assert.Error(t, err)

	cfg := cm.Config()
	assert.Equal(t, config.DefaultTheme, cfg.Proxy.Theme)
}
