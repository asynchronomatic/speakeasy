package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
