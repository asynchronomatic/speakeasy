package jsonrpc

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientDoesNotFollowRedirects(t *testing.T) {
	hit := false
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(dest.Close)

	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL+"/leaked", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(src.Close)

	c := NewClient(src.URL, "")
	err := c.Post("/api/v1/redeem/x", map[string]string{"k": "v"}, nil)
	if err == nil {
		t.Fatal("expected redirect error")
	}
	if !errors.Is(err, ErrRedirectsDisabled) {
		t.Fatalf("err=%v want redirects disabled", err)
	}
	if hit {
		t.Fatal("followed redirect")
	}
}

func TestClientErrorOmitsBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret-from-imds", http.StatusGone)
	}))
	t.Cleanup(ts.Close)

	c := NewClient(ts.URL, "")
	err := c.Get("/x", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "410") {
		t.Fatalf("err=%v want status 410", err)
	}
	if strings.Contains(err.Error(), "secret-from-imds") {
		t.Fatalf("error leaked body: %v", err)
	}
}

func TestClientOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	t.Cleanup(ts.Close)

	c := NewClient(ts.URL, "")
	var out map[string]string
	if err := c.Get("/x", &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != "yes" {
		t.Fatalf("%+v", out)
	}
}

func TestClientClosesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"a":1}`)
	}))
	t.Cleanup(ts.Close)
	c := NewClient(ts.URL, "")
	if err := c.Get("/x", &struct{ A int }{}); err != nil {
		t.Fatal(err)
	}
}
