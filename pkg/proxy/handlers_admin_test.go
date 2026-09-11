package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/testable"
)

var ProxyLoginSecret = "test-password"

const testDefaultConfigYAML = `
proxy:
  listen: ":0"
  password: test-password
admin:
  secret: s 
mesh:
  name: box
  address: http://10.0.0.1:4002
providers:
- id: local
  type: ollama
  base_url: http://127.0.0.1:11434
  token: secret-local
  private: false
  model_discovery: pinned
`

func testProxy(t *testing.T) *Proxy {
	t.Helper()
	return newTestProxy(t, testable.MustConfigManager(testDefaultConfigYAML))
}

func newTestProxy(t *testing.T, cm *testable.ConfigManager) *Proxy {
	t.Helper()

	orch := testable.NewMeshOrchestrator()
	p, err := NewProxy(orch.NewMeshNode("000001", "left"), cm)
	if err != nil {
		t.Fatal(err)
	}

	return p
}
func doProxyJSONNoLogin(t *testing.T, p *Proxy, method, path string, in, out any) error {
	t.Helper()
	return testable.NewProxyClient("proxy", p.ServeHTTP).Do(method, path, in, out)
}

func doProxyJSON(t *testing.T, p *Proxy, method, path string, in, out any) error {
	t.Helper()
	client := testable.NewProxyClient("proxy", p.ServeHTTP)
	err := client.Login("admin", ProxyLoginSecret)
	assert.NoError(t, err)
	return client.Do(method, path, in, out)
}

func getLoginToken(t *testing.T, p *Proxy, secret string) string {
	t.Helper()
	token, err := testable.NewProxyClient("proxy", p.ServeHTTP).LoginGetToken("admin", secret)
	assert.NoError(t, err)
	return token
}

func TestSecurityHeaders(t *testing.T) {
	p := newTestProxy(t, testable.MustConfigManager(testConfigYAML))

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ProxyLoginSecret))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	res := rec.Result()
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
	if got := res.Header.Get("Permissions-Policy"); !strings.Contains(got, "camera=()") {
		t.Fatalf("Permissions-Policy %q", got)
	}
	csp := res.Header.Get("Content-Security-Policy")
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

func TestMeshAPIRequiresLogin(t *testing.T) {
	p := testProxy(t)

	err := doProxyJSONNoLogin(t, p, http.MethodGet, "/api/mesh/models", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, statusCode(err))

	err = doProxyJSON(t, p, http.MethodGet, "/api/mesh/models", nil, nil)
	assert.NoError(t, err)
}

func TestMeshLoginIssuesSessionToken(t *testing.T) {
	p := testProxy(t)

	err := doProxyJSON(t, p, http.MethodPost, "/api/mesh/login", map[string]string{
		"user":     "admin",
		"password": "wrong",
	}, nil)
	assert.Equal(t, http.StatusUnauthorized, statusCode(err))

	var got struct {
		Token string `json:"token"`
	}

	err = doProxyJSON(t, p, http.MethodPost, "/api/mesh/login", map[string]string{
		"user":     "admin",
		"password": ProxyLoginSecret,
	}, &got)
	assert.NoError(t, err)

	if got.Token == "" || got.Token == ProxyLoginSecret {
		t.Fatalf("token %q", got.Token)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mesh/models", nil)
	req.Header.Set("Authorization", "Bearer "+got.Token)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models with session %d want 200", rec.Code)
	}
}

func TestAdminEnabledOff(t *testing.T) {
	p := testProxy(t)

	got := struct {
		Enabled bool `json:"enabled"`
	}{}

	err := doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil, &got)
	assert.NoError(t, err)
	assert.False(t, got.Enabled)
}

func TestAdminEnableToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/invite", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ListInvitesResponse{Invites: []api.InviteInfo{}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cm := testable.MustConfigManager(testDefaultConfigYAML)

	orch := testable.NewMeshOrchestrator()
	orch.SetAdminAddress(ts.URL)
	p, err := NewProxy(orch.NewMeshNode("000001", "left"), cm)
	if err != nil {
		t.Fatal(err)
	}

	err = doProxyJSON(t, p, http.MethodPost, "/api/admin/enabled", map[string]string{"token": ""}, nil)
	assert.Equal(t, http.StatusBadRequest, statusCode(err))

	err = doProxyJSON(t, p, http.MethodPost, "/api/admin/enabled", map[string]string{"token": "bad-token"}, nil)
	assert.Equal(t, http.StatusPreconditionFailed, statusCode(err))

	var status struct {
		Enabled bool `json:"enabled"`
	}

	err = doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil, &status)
	assert.NoError(t, err)
	assert.Equal(t, false, status.Enabled)

	cfg := cm.Config()
	assert.Equal(t, "s", cfg.Admin.Secret)

	err = doProxyJSON(t, p, http.MethodPost, "/api/admin/enabled", map[string]string{"token": "good-token"}, &status)
	assert.NoError(t, err)
	assert.Equal(t, true, status.Enabled)

	err = doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil, &status)
	assert.NoError(t, err)
	assert.Equal(t, true, status.Enabled)

	cfg = cm.Config()
	assert.Equal(t, "good-token", cfg.Admin.Secret)
	assert.Equal(t, "box", cfg.Mesh.Name)
	assert.Equal(t, 1, len(cfg.Providers))
	assert.Equal(t, "local", cfg.Providers[0].ID)
}

func TestAdminEnabledOn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	var got struct {
		Enabled bool `json:"enabled"`
	}

	p := testProxy(t)
	p.WithAdminController(api.NewClient(upstream.URL, "secret").Admin())
	err := doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil, &got)
	assert.NoError(t, err)
	assert.True(t, got.Enabled)
}

func TestAdminInvitesUnavailable(t *testing.T) {
	p := testProxy(t)
	err := doProxyJSON(t, p, http.MethodGet, "/api/admin/invite", nil, nil)
	assert.Equal(t, http.StatusServiceUnavailable, statusCode(err))
}

func TestAdminInviteCRUD(t *testing.T) {
	var stored []api.InviteInfo
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/admin/invite", func(w http.ResponseWriter, r *http.Request) {
		var req api.CreateInviteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		inv := api.InviteInfo{
			InviteId:   "abc123",
			InviteLink: "http://admin/api/v1/redeem/abc123",
			Name:       req.Name,
			Reusable:   req.Reusable,
			Expires:    0,
			MeshId:     req.MeshId,
		}
		stored = append(stored, inv)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.CreateInviteResponse{
			InviteId:   inv.InviteId,
			InviteLink: inv.InviteLink,
		})
	})
	mux.HandleFunc("GET /api/v1/admin/invite", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ListInvitesResponse{Invites: stored})
	})
	mux.HandleFunc("DELETE /api/v1/admin/invite/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		next := stored[:0]
		for _, inv := range stored {
			if inv.InviteId != id {
				next = append(next, inv)
			}
		}
		stored = next
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.DeleteInviteRequest{Invite: id})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	p := testProxy(t)
	p.WithAdminController(api.NewClient(ts.URL, "secret").Admin())

	var created api.CreateInviteResponse
	err := doProxyJSON(t, p, http.MethodPost, "/api/admin/invite", api.CreateInviteRequest{
		Name:        "guest",
		Reusable:    false,
		LifetimeSec: api.DefaultInviteLifetimeSec,
	}, &created)
	assert.NoError(t, err)
	assert.Equal(t, "abc123", created.InviteId)
	assert.NotEqual(t, "", created.InviteLink)

	var listed api.ListInvitesResponse
	err = doProxyJSON(t, p, http.MethodGet, "/api/admin/invite", nil, &listed)
	assert.NoError(t, err)
	assert.Len(t, listed.Invites, 1)
	assert.Equal(t, "guest", listed.Invites[0].Name)
	assert.False(t, listed.Invites[0].Reusable)

	err = doProxyJSON(t, p, http.MethodDelete, "/api/admin/invite/abc123", nil, nil)
	assert.NoError(t, err)

	err = doProxyJSON(t, p, http.MethodGet, "/api/admin/invite", nil, &listed)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(listed.Invites))

}

func TestAdminNodesUnavailable(t *testing.T) {
	p := testProxy(t)

	err := doProxyJSON(t, p, http.MethodGet, "/api/admin/node", nil, nil)
	assert.Equal(t, http.StatusServiceUnavailable, statusCode(err))

	err = doProxyJSON(t, p, http.MethodDelete, "/api/admin/node/peer-1", nil, nil)
	assert.Equal(t, http.StatusServiceUnavailable, statusCode(err))
}

func TestAdminNodeListAndKick(t *testing.T) {
	nodes := []api.AdminNode{{ID: "peer-1", Name: "n1", MeshId: "default"}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ListAdminNodesResponse{Nodes: nodes})
	})
	mux.HandleFunc("DELETE /api/v1/admin/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		next := nodes[:0]
		for _, n := range nodes {
			if n.ID != id {
				next = append(next, n)
			}
		}
		nodes = next
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.KickPeerResponse{NodeID: id})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	p := testProxy(t)
	p.WithAdminController(api.NewClient(ts.URL, "secret").Admin())

	var listed api.ListAdminNodesResponse
	err := doProxyJSON(t, p, http.MethodGet, "/api/admin/node", nil, &listed)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(listed.Nodes))
	assert.Equal(t, "peer-1", listed.Nodes[0].ID)

	var kicked api.KickPeerResponse
	err = doProxyJSON(t, p, http.MethodDelete, "/api/admin/node/peer-1", nil, &kicked)
	assert.NoError(t, err)
	assert.Equal(t, "peer-1", kicked.NodeID)

	err = doProxyJSON(t, p, http.MethodGet, "/api/admin/node", nil, &listed)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(listed.Nodes))
}
