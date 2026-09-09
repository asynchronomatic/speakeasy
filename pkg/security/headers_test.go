package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SetHeaders(rec)
	h := rec.Result().Header
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options %q", got)
	}
	if got := h.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options %q", got)
	}
	if got := h.Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy %q", got)
	}
	if got := h.Get("Permissions-Policy"); !strings.Contains(got, "camera=()") {
		t.Fatalf("Permissions-Policy %q", got)
	}
	csp := h.Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP missing %q in %q", want, csp)
		}
	}
}

func TestHandlerSetsHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	rec := httptest.NewRecorder()
	Handler(inner).ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", rec.Result().StatusCode)
	}
	if rec.Result().Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("wrapper did not set headers")
	}
}
