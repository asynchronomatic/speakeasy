package socket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func startNotifier(t *testing.T) (*Notifier, *httptest.Server, string) {
	t.Helper()
	n := NewNotifier()
	go n.Poll()
	srv := httptest.NewServer(http.HandlerFunc(n.Handle))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	return n, srv, wsURL
}

func TestNotifierBroadcastWakeup(t *testing.T) {
	n, srv, wsURL := startNotifier(t)
	origin := srv.URL
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {origin}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n.Broadcast()
		_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			continue
		}
		if string(msg) != "wakeup" {
			t.Fatalf("got %q want wakeup", msg)
		}
		return
	}
	t.Fatal("timed out waiting for wakeup")
}

func TestNotifierRejectsCrossOrigin(t *testing.T) {
	_, _, wsURL := startNotifier(t)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {"http://evil.example"}})
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected cross-origin upgrade to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %+v want 403", resp)
	}
}

func TestNotifierRejectsCrossSiteFetch(t *testing.T) {
	_, srv, wsURL := startNotifier(t)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Origin":         {srv.URL},
		"Sec-Fetch-Site": {"cross-site"},
	})
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected cross-site upgrade to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %+v want 403", resp)
	}
}
