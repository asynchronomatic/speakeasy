package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsJSONContentType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"APPLICATION/JSON", true},
		{"text/plain", false},
		{"application/x-www-form-urlencoded", false},
		{"", false},
		{"application/jsonn", false},
	} {
		if got := IsJSONContentType(tc.ct); got != tc.want {
			t.Fatalf("IsJSONContentType(%q)=%v want %v", tc.ct, got, tc.want)
		}
	}
}

func TestRequireJSONContentType(t *testing.T) {
	t.Parallel()
	good := httptest.NewRequest(http.MethodPost, "/x", nil)
	good.Header.Set("Content-Type", "application/json")
	if err := RequireJSONContentType(good); err != nil {
		t.Fatalf("json: %v", err)
	}

	bad := httptest.NewRequest(http.MethodPost, "/x", nil)
	bad.Header.Set("Content-Type", "text/plain")
	err := RequireJSONContentType(bad)
	if err == nil {
		t.Fatal("expected error")
	}
	ce, isErr := err.(*Error)
	if !isErr || ce.Code() != http.StatusUnsupportedMediaType {
		t.Fatalf("got %v", err)
	}
}

func TestRequireSameOrigin(t *testing.T) {
	t.Parallel()
	same := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4080/api/mesh/providers", nil)
	same.Host = "127.0.0.1:4080"
	same.Header.Set("Origin", "http://127.0.0.1:4080")
	if err := RequireSameOrigin(same); err != nil {
		t.Fatalf("same origin: %v", err)
	}

	none := httptest.NewRequest(http.MethodPost, "/api/mesh/providers", nil)
	if err := RequireSameOrigin(none); err != nil {
		t.Fatalf("no origin: %v", err)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/mesh/providers", nil)
	get.Header.Set("Origin", "http://evil.example")
	if err := RequireSameOrigin(get); err != nil {
		t.Fatalf("safe method: %v", err)
	}

	cross := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4080/api/mesh/providers", nil)
	cross.Host = "127.0.0.1:4080"
	cross.Header.Set("Origin", "http://evil.example")
	err := RequireSameOrigin(cross)
	if err == nil {
		t.Fatal("expected origin mismatch")
	}
	ce, ok := err.(*Error)
	if !ok || ce.Code() != http.StatusForbidden {
		t.Fatalf("got %v", err)
	}
}

func TestOriginOK(t *testing.T) {
	t.Parallel()
	same := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4080/api/v.1/refresh/websocket", nil)
	same.Host = "127.0.0.1:4080"
	same.Header.Set("Origin", "http://127.0.0.1:4080")
	if !OriginOK(same) {
		t.Fatal("same origin GET should be allowed")
	}

	none := httptest.NewRequest(http.MethodGet, "/api/v.1/refresh/websocket", nil)
	if !OriginOK(none) {
		t.Fatal("missing origin should be allowed")
	}

	cross := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4080/api/v.1/refresh/websocket", nil)
	cross.Host = "127.0.0.1:4080"
	cross.Header.Set("Origin", "http://evil.example")
	if OriginOK(cross) {
		t.Fatal("cross origin GET should be denied")
	}
}
