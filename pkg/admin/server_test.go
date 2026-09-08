package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
)

func testNewServer(t *testing.T, addr, secret string) *Server {
	t.Helper()
	t.Setenv("ADMIN_DB_PATH", filepath.Join(t.TempDir(), "admin.jkv"))
	s, err := NewServer(addr, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAdminSecurityHeaders(t *testing.T) {
	s := testNewServer(t, ":0", "test-secret")
	ts := httptest.NewUnstartedServer(s.routes())
	ts.Config.SetKeepAlivesEnabled(false)
	ts.Start()
	t.Cleanup(func() {
		ts.Close()
		closeIdleHTTP()
	})

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options %q", got)
	}
	if got := res.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options %q", got)
	}
	if got := res.Header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy %q", got)
	}
	csp := res.Header.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "base-uri 'none'", "form-action 'self'", "script-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP missing %q in %q", want, csp)
		}
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	s := testNewServer(t, ":0", "test-secret")
	ts := httptest.NewUnstartedServer(s.routes())
	ts.Config.SetKeepAlivesEnabled(false)
	ts.Start()
	t.Cleanup(func() {
		ts.Close()
		closeIdleHTTP()
	})

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /nodes: got %d", res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	res, err = testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("token GET /nodes: got %d", res.StatusCode)
	}
}

func TestNewServerUsesAdminDBPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom.jkv")
	t.Setenv("ADMIN_DB_PATH", dir)
	s, err := NewServer(":0", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.kv.Put("probe", "ok"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	kv, err := jsonkv.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kv.Close() })
	var got string
	if err := kv.Get("probe", &got); err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("got %q", got)
	}
}

func TestAdminDBPathDefault(t *testing.T) {
	t.Setenv("ADMIN_DB_PATH", "")
	if got := adminDBPath(); got != "admin.jkv" {
		t.Fatalf("adminDBPath()=%q", got)
	}
	t.Setenv("ADMIN_DB_PATH", " /custom/path ")
	if got := adminDBPath(); got != "/custom/path" {
		t.Fatalf("adminDBPath()=%q", got)
	}
}

func TestAdminAllowPath(t *testing.T) {
	t.Setenv("ADMIN_ALLOW_PATH", "")
	t.Setenv("ADMIN_DB_PATH", "")
	if got := adminAllowPath(); got != "allow.list" {
		t.Fatalf("default adminAllowPath()=%q", got)
	}

	t.Setenv("ADMIN_DB_PATH", "/var/lib/speakeasy/admin.jkv")
	if got := adminAllowPath(); got != "/var/lib/speakeasy/allow.list" {
		t.Fatalf("colocated adminAllowPath()=%q", got)
	}

	t.Setenv("ADMIN_ALLOW_PATH", " /etc/speakeasy/allow.list ")
	if got := adminAllowPath(); got != "/etc/speakeasy/allow.list" {
		t.Fatalf("override adminAllowPath()=%q", got)
	}
}

func TestNewServerUsesAllowListNextToDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "admin.jkv")
	allowPath := filepath.Join(dir, "allow.list")
	if err := os.WriteFile(allowPath, []byte("peer-seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADMIN_DB_PATH", dbPath)
	t.Setenv("ADMIN_ALLOW_PATH", "")

	s, err := NewServer(":0", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !s.acl.Has("peer-seed") {
		t.Fatal("expected seed peer from colocated allow.list")
	}
	s.acl.Add("peer-added")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewAllowList(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Has("peer-seed") || !reloaded.Has("peer-added") {
		t.Fatalf("persisted peers: %v", reloaded.Peers())
	}
}

func TestNewServerAllowListOpenError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ADMIN_DB_PATH", filepath.Join(dir, "admin.jkv"))
	t.Setenv("ADMIN_ALLOW_PATH", dir)
	if _, err := NewServer(":0", "test-secret"); err == nil {
		t.Fatal("expected error opening directory as allow list")
	}
}

func TestAdminBearerAuthAndRegister(t *testing.T) {
	s := testNewServer(t, ":0", "test-secret")
	ts := httptest.NewUnstartedServer(s.routes())
	ts.Config.SetKeepAlivesEnabled(false)
	ts.Start()
	t.Cleanup(func() {
		ts.Close()
		closeIdleHTTP()
	})

	body, _ := json.Marshal(api.RegisterNodeRequest{Node: api.Node{Name: "n1", ID: "peer-1"}})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/authorize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("Content-Type", "application/json")
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authorize: got %d", res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/nodes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("Content-Type", "application/json")
	res, err = testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d", res.StatusCode)
	}
}
