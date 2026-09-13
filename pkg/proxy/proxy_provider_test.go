package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/testable"
)

func testLocalProvider(t *testing.T, backendURL, token string) *Proxy {
	t.Helper()

	cm := testable.MustConfigManager(testDefaultConfigYAML)
	cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.InferenceTokens.Insecure = true
		cfg.Proxy.AllowPrivateBackends = true
		cfg.Providers = []config.Provider{{
			ID:        "local",
			Type:      "test",
			BaseURL:   backendURL,
			Token:     token,
			Discovery: "whitelist",
			Models: []config.ModelConfig{{
				Model: "echo",
			}},
		}}
		return nil
	})

	return newTestProxy(t, cm)

}

func TestReverseProxyStripsClientAuth(t *testing.T) {
	var gotAuth, gotCookie, gotKey, gotOrigin string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		gotKey = r.Header.Get("X-Api-Key")
		gotOrigin = r.Header.Get("Origin")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(backend.Close)

	p := testLocalProvider(t, backend.URL, "")
	body := `{"model":"echo","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-secret")
	req.Header.Set("Cookie", "session=abc")
	req.Header.Set("X-Api-Key", "client-key")
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if gotAuth != "" || gotCookie != "" || gotKey != "" || gotOrigin != "" {
		t.Fatalf("forwarded auth=%q cookie=%q key=%q origin=%q", gotAuth, gotCookie, gotKey, gotOrigin)
	}
}

func TestReverseProxySetsProviderToken(t *testing.T) {
	var gotAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(backend.Close)

	p := testLocalProvider(t, backend.URL, "prov-tok")
	body := `{"model":"echo","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-secret")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer prov-tok" {
		t.Fatalf("auth %q", gotAuth)
	}
}

func TestReverseProxyRejectsRedirect(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	t.Cleanup(backend.Close)

	p := testLocalProvider(t, backend.URL, "")
	rec := postModel(t, p, "/v1/chat/completions", "echo")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d want 502 body %s", rec.Code, rec.Body.String())
	}
}

func TestReverseProxyRejectsMetadataURL(t *testing.T) {
	cm := testable.MustConfigManager(testDefaultConfigYAML)
	cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Providers = []config.Provider{{
			ID:        "meta",
			Type:      "test",
			BaseURL:   "http://169.254.169.254/",
			Discovery: "whitelist",
			Models:    []config.ModelConfig{{Model: "echo"}},
		}}
		return nil
	})
	p := newTestProxy(t, cm)
	rec := postModel(t, p, "/v1/chat/completions", "echo")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d want 502 body %s", rec.Code, rec.Body.String())
	}
}
