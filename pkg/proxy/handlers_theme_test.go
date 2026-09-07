package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

func decodeTheme(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	var body themeResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Theme
}

func TestThemeGetDefaultsDeco(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil)
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	if got := decodeTheme(t, res); got != core.DefaultTheme {
		t.Fatalf("theme %q want %q", got, core.DefaultTheme)
	}
}

func TestThemeGetStored(t *testing.T) {
	writeTestConfig(t, `proxy:
  listen: ":4080"
  theme: clean
mesh:
  address: http://x
`)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if got := decodeTheme(t, res); got != "clean" {
		t.Fatalf("theme %q", got)
	}
}

func TestThemeGetInvalidDefaultsDeco(t *testing.T) {
	writeTestConfig(t, `proxy:
  listen: ":4080"
  theme: nope
mesh:
  address: http://x
`)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil)
	if got := decodeTheme(t, res); got != core.DefaultTheme {
		t.Fatalf("theme %q want %q", got, core.DefaultTheme)
	}
}

func TestThemeSetPersists(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", themeResponse{Theme: "Clean"})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	if got := decodeTheme(t, res); got != "clean" {
		t.Fatalf("reply %q", got)
	}

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

	res = doProxyJSON(t, p, http.MethodGet, "/api/mesh/theme", nil)
	if got := decodeTheme(t, res); got != "clean" {
		t.Fatalf("get after set %q", got)
	}
}

func TestThemeSetEmptyDefaultsDeco(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", themeResponse{Theme: ""})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	if got := decodeTheme(t, res); got != core.DefaultTheme {
		t.Fatalf("reply %q", got)
	}
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
	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/theme", themeResponse{Theme: "neon"})
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d want 400", res.StatusCode)
	}
	cfg, err := core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Theme != "" {
		t.Fatalf("invalid set wrote %q", cfg.Proxy.Theme)
	}
}
