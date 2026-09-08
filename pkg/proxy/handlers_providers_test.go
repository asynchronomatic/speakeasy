package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

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

func decodeProviderList(t *testing.T, res *http.Response) []core.Provider {
	t.Helper()
	defer res.Body.Close()
	var body providersListResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Providers
}

func TestProvidersList(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/providers", nil)
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	got := decodeProviderList(t, res)
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
	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/providers", nil)
	got := decodeProviderList(t, res)
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty list, got %+v", got)
	}
}

func TestProviderAddUpdateDelete(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)

	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{
		ID:        "cloud",
		Type:      "openai",
		BaseURL:   "https://api.example",
		Token:     "tok",
		Private:   true,
		Discovery: "whitelist",
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("add %d: %s", res.StatusCode, b)
	}
	var added core.Provider
	if err := json.NewDecoder(res.Body).Decode(&added); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if added.ID != "cloud" || added.BaseURL != "https://api.example" || !added.Private {
		t.Fatalf("added %+v", added)
	}
	if added.Token != "" {
		t.Fatalf("add leaked token %q", added.Token)
	}

	cfg, err := core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mesh.Address != "http://10.0.0.1:4002" || cfg.Proxy.Listen != ":4080" {
		t.Fatalf("non-provider fields changed: %+v", cfg)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("providers %+v", cfg.Providers)
	}
	local, cloud := cfg.Providers[0], cfg.Providers[1]
	if local.ID != "local" {
		local, cloud = cloud, local
	}
	if local.Token != "secret-local" {
		t.Fatalf("add wiped local token: %+v", cfg.Providers)
	}
	if cloud.ID != "cloud" || cloud.Token != "tok" {
		t.Fatalf("added provider on disk %+v", cloud)
	}

	res = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/cloud", core.Provider{
		Type:      "openai",
		BaseURL:   "https://api.example/v1",
		Token:     "new-tok",
		Private:   false,
		Discovery: "all",
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("update %d: %s", res.StatusCode, b)
	}
	var updated core.Provider
	if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if updated.ID != "cloud" || updated.BaseURL != "https://api.example/v1" || updated.Private {
		t.Fatalf("updated %+v", updated)
	}
	if updated.Token != "" {
		t.Fatalf("update leaked token %q", updated.Token)
	}

	res = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/cloud", core.Provider{
		Type:      "openai",
		BaseURL:   "https://api.example/v1",
		Token:     "*",
		Private:   false,
		Discovery: "all",
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("keep-token update %d: %s", res.StatusCode, b)
	}
	res.Body.Close()

	cfg, err = core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	var gotCloud core.Provider
	for _, pr := range cfg.Providers {
		if pr.ID == "cloud" {
			gotCloud = pr
		}
		if pr.ID == "local" && pr.Token != "secret-local" {
			t.Fatalf("local token after update %q", pr.Token)
		}
	}
	if gotCloud.Token != "new-tok" {
		t.Fatalf("cloud token after keep update %q", gotCloud.Token)
	}

	res = doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/cloud", core.Provider{
		Type:      "openai",
		BaseURL:   "https://api.example/v1",
		Token:     "",
		Private:   false,
		Discovery: "all",
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("empty-token update %d: %s", res.StatusCode, b)
	}
	res.Body.Close()
	cfg, err = core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	for _, pr := range cfg.Providers {
		if pr.ID == "cloud" && pr.Token != "new-tok" {
			t.Fatalf("empty token overwrote cloud token %q", pr.Token)
		}
	}

	listed := decodeProviderList(t, doProxyJSON(t, p, http.MethodGet, "/api/mesh/providers", nil))
	for _, pr := range listed {
		if pr.Token != "" {
			t.Fatalf("list leaked token for %s: %q", pr.ID, pr.Token)
		}
	}

	res = doProxyJSON(t, p, http.MethodDelete, "/api/mesh/providers/cloud", nil)
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("delete %d: %s", res.StatusCode, b)
	}
	res.Body.Close()

	cfg, err = core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "local" {
		t.Fatalf("after delete %+v", cfg.Providers)
	}
	if cfg.Providers[0].Token != "secret-local" {
		t.Fatalf("delete wiped local token %q", cfg.Providers[0].Token)
	}
	if cfg.Mesh.Name != "box" {
		t.Fatalf("mesh name %q", cfg.Mesh.Name)
	}
}

func TestProviderAddDuplicate(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{
		ID:      "local",
		Type:    "ollama",
		BaseURL: "http://127.0.0.1:11434",
	})
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status %d", res.StatusCode)
	}
}

func TestProviderAddRequiresFields(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers", core.Provider{Type: "ollama", BaseURL: "http://x"})
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing id status %d", res.StatusCode)
	}
}

func TestProviderUpdateMissing(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodPost, "/api/mesh/providers/nope", core.Provider{
		Type:    "ollama",
		BaseURL: "http://x",
	})
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing status %d", res.StatusCode)
	}
}

func TestProviderDeleteMissing(t *testing.T) {
	writeTestConfig(t, testConfigYAML)
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodDelete, "/api/mesh/providers/nope", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing status %d", res.StatusCode)
	}
}
