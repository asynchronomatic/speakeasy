package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/testable"
)

func TestRefreshTicketRequiresAuth(t *testing.T) {
	p := testProxy(t)

	client := testable.NewProxyClient("proxy", p.ServeHTTP)

	err := client.Do(http.MethodPost, "/api/mesh/refresh/ticket", map[string]any{}, nil)
	assert.Equal(t, http.StatusUnauthorized, statusCode(err))
}

func TestRefreshTicketSetsCookie(t *testing.T) {
	p := testProxy(t)

	client := testable.NewProxyClient("proxy", p.ServeHTTP)
	token, err := client.LoginGetToken("admin", ProxyLoginSecret)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	req := httptest.NewRequest(http.MethodPost, "/api/mesh/refresh/ticket", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res := client.DoRaw(req)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	var body map[string]bool
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body["ok"] {
		t.Fatalf("body %+v", body)
	}
	c := cookieNamed(res.Cookies(), wsTicketCookie)
	if c == nil || c.Value == "" {
		t.Fatal("missing ticket cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags %+v", c)
	}
	if c.Path != "/api/v.1/refresh/websocket" {
		t.Fatalf("cookie path %q", c.Path)
	}
	if c.Secure {
		t.Fatal("Secure should be off on HTTP")
	}
}

func TestWebsocketQueryTokenRejected(t *testing.T) {
	p := testProxy(t)

	client := testable.NewProxyClient("proxy", p.ServeHTTP)
	token, err := client.LoginGetToken("admin", ProxyLoginSecret)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	req := httptest.NewRequest(http.MethodGet, "/api/v.1/refresh/websocket?access_token="+token, nil)

	res := client.DoRaw(req)
	require.NotNil(t, res)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestWebsocketUpgradeWithTicket(t *testing.T) {
	p := testProxy(t)

	go p.notifier.Poll()
	ts := httptest.NewServer(p)
	t.Cleanup(ts.Close)

	client := testable.NewProxyClient("proxy", p.ServeHTTP)
	token, err := client.LoginGetToken("admin", ProxyLoginSecret)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/mesh/refresh/ticket", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ticket status %d", res.StatusCode)
	}
	c := cookieNamed(res.Cookies(), wsTicketCookie)
	if c == nil {
		t.Fatal("missing ticket cookie")
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v.1/refresh/websocket"
	hdr := http.Header{}
	hdr.Set("Cookie", c.Name+"="+c.Value)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial: %v status %d", err, resp.StatusCode)
		}
		t.Fatal(err)
	}
	conn.Close()

	conn, resp, err = websocket.DefaultDialer.Dial(wsURL, hdr)
	if err == nil {
		conn.Close()
		t.Fatal("reused ticket should fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reuse status %v", resp)
	}
}

func TestWebsocketUpgradeWithBearer(t *testing.T) {
	p := testProxy(t)

	go p.notifier.Poll()
	ts := httptest.NewServer(p)
	t.Cleanup(ts.Close)

	client := testable.NewProxyClient("proxy", p.ServeHTTP)
	token, err := client.LoginGetToken("admin", ProxyLoginSecret)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v.1/refresh/websocket"
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}

func TestWSTicketExpires(t *testing.T) {
	p := testProxy(t)
	raw, err := p.issueWSTicket()
	if err != nil {
		t.Fatal(err)
	}
	p.wsTicketMu.Lock()
	p.wsTickets[hashWSTicket(raw)] = time.Now().Add(-time.Second)
	p.wsTicketMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v.1/refresh/websocket", nil)
	req.AddCookie(&http.Cookie{Name: wsTicketCookie, Value: raw})
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired ticket status %d", rec.Code)
	}
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
