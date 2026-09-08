package mesh

import (
	"net/http"
	"testing"

	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func TestScrubMeshRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://peer.mesh/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer proxy-password")
	req.Header.Set("Proxy-Authorization", "Basic abc")
	req.Header.Set("Cookie", "speakeasy_ws=ticket")
	req.Header.Set("X-Api-Key", "cloud-key")
	req.Header.Set("Origin", "http://127.0.0.1:4080")
	req.Header.Set("Connection", "keep-alive, X-Hop")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Proxy-Connection", "keep-alive")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("TE", "trailers")
	req.Header.Set("Trailer", "X-Checksum")
	req.Header.Set("X-Hop", "should-drop")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "keep-me")

	security.ScrubHeaders(req, security.DefaultAllowedHeaders)

	for _, name := range []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"X-Api-Key",
		"Origin",
		"Connection",
		"Keep-Alive",
		"Proxy-Connection",
		"Transfer-Encoding",
		"Upgrade",
		"TE",
		"Trailer",
		"X-Hop",
	} {
		if got := req.Header.Get(name); got != "" {
			t.Errorf("%s still set: %q", name, got)
		}
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type=%q", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("X-Request-Id") != "keep-me" {
		t.Fatalf("X-Request-Id=%q", req.Header.Get("X-Request-Id"))
	}
}
