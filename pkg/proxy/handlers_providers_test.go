package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

func writeTestConfig(t *testing.T, yaml string) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
}

const testConfigYAML = `proxy:
  listen: ":4080"
admin:
  secret: s
mesh:
  name: box
  address: http://10.0.0.1:4002
providers:
- id: local
  type: ollama
  base_url: http://127.0.0.1:11434
  token: secret-local
  private: false
  model_discovery: pinned
`

func TestProvidersList(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	resp := providersListResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/providers", nil, &resp)
	assert.NoError(t, err)

	got := resp.Providers
	if len(got) != 1 || got[0].ID != "local" || got[0].Type != "ollama" {
		t.Fatalf("list %+v", got)
	}
	if got[0].Token != "" {
		t.Fatalf("list leaked token %q", got[0].Token)
	}
}

func TestProvidersListEmpty(t *testing.T) {
	writeTestConfig(t, `proxy:
  listen: ":1"
mesh:
  address: http://x
`)
	p := testProxy(t)
	resp := providersListResponse{}
	err := doProxyJSON(t, p, http.MethodGet, "/api/mesh/providers", nil, &resp)
	assert.NoError(t, err)

	got := resp.Providers
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty list, got %+v", got)
	}
}

func TestProviderAddUpdateDelete(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	var added core.Provider
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{
		ID:        "cloud",
		Type:      "openai",
		BaseURL:   "https://api.example",
		Token:     "tok",
		Private:   true,
		Discovery: "whitelist",
	}, &added)
	assert.NoError(t, err)
	assert.Equal(t, "cloud", added.ID)
	assert.Equal(t, "openai", added.Type)
	assert.Equal(t, "https://api.example", added.BaseURL)
	assert.Equal(t, "", added.Token)
	assert.Equal(t, true, added.Private)
	assert.Equal(t, "whitelist", added.Discovery)

	cfg, err := core.LoadConfigFile()
	require.NoError(t, err)

	assert.Equal(t, "http://10.0.0.1:4002", cfg.Mesh.Address)
	assert.Equal(t, ":4080", cfg.Proxy.Listen)
	assert.Equal(t, 2, len(cfg.Providers))

	local, cloud := cfg.Providers[0], cfg.Providers[1]
	if local.ID != "local" {
		local, cloud = cloud, local
	}
	assert.Equal(t, "secret-local", local.Token)
	assert.Equal(t, "tok", cloud.Token)

	var updated core.Provider
	err = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/cloud", core.Provider{
		Type:      "openai",
		BaseURL:   "https://api.example/v1",
		Token:     "new-tok",
		Private:   false,
		Discovery: "all",
	}, &updated)
	assert.NoError(t, err)
	assert.Equal(t, "cloud", updated.ID)
	assert.Equal(t, "openai", updated.Type)
	assert.Equal(t, "https://api.example/v1", updated.BaseURL)
	assert.Equal(t, "", updated.Token)
	assert.Equal(t, false, updated.Private)
	assert.Equal(t, "all", updated.Discovery)

	var gotCloud core.Provider
	err = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/cloud", core.Provider{
		Type:      "openai",
		BaseURL:   "https://api.example/v1",
		Token:     "*",
		Private:   false,
		Discovery: "all",
	}, &gotCloud)
	assert.NoError(t, err)

	cfg, err = core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}

	for _, pr := range cfg.Providers {
		if pr.ID == "cloud" {
			gotCloud = pr
		}
		if pr.ID == "local" && pr.Token != "secret-local" {
			t.Fatalf("local token after update %q", pr.Token)
		}
	}
	assert.Equal(t, "new-tok", gotCloud.Token)

	err = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/cloud", core.Provider{
		Type:      "openai",
		BaseURL:   "https://api.example/v1",
		Token:     "",
		Private:   false,
		Discovery: "all",
	}, nil)
	assert.NoError(t, err)

	cfg, err = core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	for _, pr := range cfg.Providers {
		if pr.ID == "cloud" && pr.Token != "new-tok" {
			t.Fatalf("empty token overwrote cloud token %q", pr.Token)
		}
	}

	var resp providersListResponse
	err = doProxyJSON(t, p, http.MethodGet, "/api/mesh/providers", nil, &resp)
	assert.NoError(t, err)

	for _, pr := range resp.Providers {
		if pr.Token != "" {
			t.Fatalf("list leaked token for %s: %q", pr.ID, pr.Token)
		}
	}

	err = doProxyJSON(t, p, http.MethodDelete, "/api/mesh/providers/cloud", nil, nil)
	assert.NoError(t, err)

	cfg, err = core.LoadConfigFile()
	require.NoError(t, err)
	require.Equal(t, 1, len(cfg.Providers))
	assert.Equal(t, "local", cfg.Providers[0].ID)
	assert.Equal(t, "secret-local", cfg.Providers[0].Token)
	assert.Equal(t, "box", cfg.Mesh.Name)
}

func TestProviderAddDuplicate(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{
		ID:      "local",
		Type:    "ollama",
		BaseURL: "http://127.0.0.1:11434",
	}, nil)
	assert.Equal(t, http.StatusConflict, statusCode(err))
}

func TestProviderAddRequiresFields(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{Type: "ollama", BaseURL: "http://x"}, nil)
	assert.Equal(t, http.StatusBadRequest, statusCode(err))
}

func TestProviderAddRejectsUnsafeURL(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := newTestProxy(t, nil, true)
	cases := []core.Provider{
		{ID: "a", Type: "ollama", BaseURL: "file:///etc/passwd"},
		{ID: "b", Type: "ollama", BaseURL: "http://169.254.169.254/"},
		{ID: "c", Type: "ollama", BaseURL: "http://metadata.google.internal/"},
		{ID: "d", Type: "ollama", BaseURL: "http://user:pass@example.com"},
	}
	for _, prov := range cases {
		err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", prov, nil)
		assert.Equal(t, http.StatusBadRequest, statusCode(err))
	}
}

func TestProviderAddRejectsPrivateWithoutOptIn(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := newTestProxy(t, nil, false)
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{
		ID:      "local2",
		Type:    "ollama",
		BaseURL: "http://127.0.0.1:11434",
	}, nil)
	assert.Equal(t, http.StatusBadRequest, statusCode(err))

	err = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{
		ID:      "cloud",
		Type:    "openai",
		BaseURL: "https://api.example",
	}, nil)
	assert.NoError(t, err)
}

func TestProviderUpdateMissing(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/nope", core.Provider{
		Type:    "ollama",
		BaseURL: "http://x",
	}, nil)
	assert.Equal(t, http.StatusNotFound, statusCode(err))
}

func TestProviderRejectsNonJSONContentType(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	token := getLoginToken(t, p, ProxyLoginSecret)
	assert.NotEqual(t, "", token)

	req := httptest.NewRequest(http.MethodPost, "/api/mesh/providers", strings.NewReader(`{"id":"x","type":"ollama","base_url":"http://x"}`))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d want 415", rec.Code)
	}
}

func TestProviderRejectsCrossOrigin(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	req := httptest.NewRequest(http.MethodPost, "/api/mesh/providers", strings.NewReader(`{"id":"x","type":"ollama","base_url":"http://x"}`))
	req.Host = "127.0.0.1:4080"
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ProxyLoginSecret))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d want 403", rec.Code)
	}
}

func TestProviderDeleteMissing(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	p.WithAdminToken(ProxyLoginSecret)
	err := doProxyJSON(t, p, http.MethodDelete, "/api/mesh/providers/nope", nil, nil)
	assert.Equal(t, http.StatusNotFound, statusCode(err))
}
