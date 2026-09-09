package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSanitizeLogStripsControls(t *testing.T) {
	in := "1.2.3.4\r\nERROR forged\tline\x00"
	got := SanitizeLog(in)
	if got != "1.2.3.4ERROR forgedline" {
		t.Fatalf("got %q", got)
	}
}

func TestClientAddrIgnoresForwardedByDefault(t *testing.T) {
	t.Setenv(trustForwardedEnv, "")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "9.9.9.9\n")
	if got := ClientAddr(req); got != "10.0.0.1:1234" {
		t.Fatalf("got %q", got)
	}
}

func TestClientAddrUsesForwardedWhenTrusted(t *testing.T) {
	t.Setenv(trustForwardedEnv, "1")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.1")
	if got := ClientAddr(req); got != "9.9.9.9" {
		t.Fatalf("got %q", got)
	}
}

func TestRequestPathSanitized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.URL.Path = "/ok\nSet-Cookie: x"
	if got := RequestPath(req); got != "/okSet-Cookie: x" {
		t.Fatalf("got %q", got)
	}
}
