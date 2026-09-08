package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/testable"
)

var ProxyLoginToken = "test-password"

func testProxy(t *testing.T) *Proxy {
	t.Helper()
	return newTestProxy(t, nil, true)
}

func newTestProxy(t *testing.T, providers []core.Provider, allowPrivate bool) *Proxy {
	t.Helper()
	orch := testable.NewMeshOrchestrator()
	p, err := NewProxy(orch.NewMeshNode("000001", "left"), ":0", providers, allowPrivate)
	if err != nil {
		t.Fatal(err)
	}

	p.WithAdminToken(ProxyLoginToken)
	return p
}

func doProxyJSON(t *testing.T, p *Proxy, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ProxyLoginToken))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec.Result()
}

func TestSecurityHeaders(t *testing.T) {
	p := testProxy(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ProxyLoginToken))
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
	csp := res.Header.Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"frame-ancestors 'none'",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP missing %q in %q", want, csp)
		}
	}
}

func TestAuthRequiredOff(t *testing.T) {
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/auth", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var got struct {
		Required bool `json:"required"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	assert.True(t, got.Required)
}

func TestAuthRequiredOn(t *testing.T) {
	p := testProxy(t)
	p.WithAdminToken("sekrit")

	res := doProxyJSON(t, p, http.MethodGet, "/api/mesh/auth", nil)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("auth status %d", res.StatusCode)
	}
	var got struct {
		Required bool `json:"required"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if !got.Required {
		t.Fatal("expected required=true")
	}

	res = doProxyJSON(t, p, http.MethodGet, "/api/mesh/models", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("models without token %d want 401", res.StatusCode)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mesh/models", nil)
	req.Header.Set("Authorization", "Bearer sekrit")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models with token %d want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v.1/refresh/websocket", nil)
	rec = httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ws without token %d want 401", rec.Code)
	}
}

func TestAdminEnabledOff(t *testing.T) {
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d want 200", res.StatusCode)
	}
	var got struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false")
	}
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

	writeTestConfig(t, testConfigYAML)

	orch := testable.NewMeshOrchestrator()
	orch.SetAdminAddress(ts.URL)
	p, err := NewProxy(orch.NewMeshNode("000001", "left"), ":0", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	p.WithAdminToken(ProxyLoginToken)

	res := doProxyJSON(t, p, http.MethodPost, "/api/admin/enabled", map[string]string{"token": ""})
	if res.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("empty token %d: %s", res.StatusCode, b)
	}
	res.Body.Close()

	res = doProxyJSON(t, p, http.MethodPost, "/api/admin/enabled", map[string]string{"token": "bad-token"})
	if res.StatusCode != http.StatusPreconditionFailed {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("bad token %d: %s", res.StatusCode, b)
	}
	res.Body.Close()

	res = doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil)
	var status struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if status.Enabled {
		t.Fatal("enabled after bad token")
	}
	cfg, err := core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Secret != "s" {
		t.Fatalf("secret changed after bad token: %q", cfg.Admin.Secret)
	}

	res = doProxyJSON(t, p, http.MethodPost, "/api/admin/enabled", map[string]string{"token": "good-token"})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("good token %d: %s", res.StatusCode, b)
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if !status.Enabled {
		t.Fatal("expected enabled=true")
	}

	res = doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil)
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if !status.Enabled {
		t.Fatal("GET enabled still false after enable")
	}

	cfg, err = core.LoadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Secret != "good-token" {
		t.Fatalf("secret not saved: %q", cfg.Admin.Secret)
	}
	if cfg.Mesh.Name != "box" || len(cfg.Providers) != 1 || cfg.Providers[0].ID != "local" {
		t.Fatalf("other config fields changed: %+v", cfg)
	}
}

func TestAdminEnabledOn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	p := testProxy(t)
	p.WithAdminController(api.NewClient(upstream.URL, "secret").Admin())
	res := doProxyJSON(t, p, http.MethodGet, "/api/admin/enabled", nil)
	defer res.Body.Close()
	var got struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
}

func TestAdminInvitesUnavailable(t *testing.T) {
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/admin/invite", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("list status %d", res.StatusCode)
	}
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

	res := doProxyJSON(t, p, http.MethodPost, "/api/admin/invite", api.CreateInviteRequest{
		Name:        "guest",
		Reusable:    false,
		LifetimeSec: api.DefaultInviteLifetimeSec,
	})
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("create %d: %s", res.StatusCode, b)
	}
	var created api.CreateInviteResponse
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if created.InviteId != "abc123" || created.InviteLink == "" {
		t.Fatalf("created %+v", created)
	}

	res = doProxyJSON(t, p, http.MethodGet, "/api/admin/invite", nil)
	var listed api.ListInvitesResponse
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if len(listed.Invites) != 1 || listed.Invites[0].Name != "guest" || listed.Invites[0].Reusable {
		t.Fatalf("listed %+v", listed.Invites)
	}

	res = doProxyJSON(t, p, http.MethodDelete, "/api/admin/invite/abc123", nil)
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("revoke %d: %s", res.StatusCode, b)
	}
	res.Body.Close()

	res = doProxyJSON(t, p, http.MethodGet, "/api/admin/invite", nil)
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if len(listed.Invites) != 0 {
		t.Fatalf("after revoke %+v", listed.Invites)
	}
}

func TestAdminNodesUnavailable(t *testing.T) {
	p := testProxy(t)
	res := doProxyJSON(t, p, http.MethodGet, "/api/admin/node", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("list status %d", res.StatusCode)
	}
	res = doProxyJSON(t, p, http.MethodDelete, "/api/admin/node/peer-1", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("kick status %d", res.StatusCode)
	}
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

	res := doProxyJSON(t, p, http.MethodGet, "/api/admin/node", nil)
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("list %d: %s", res.StatusCode, b)
	}
	var listed api.ListAdminNodesResponse
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if len(listed.Nodes) != 1 || listed.Nodes[0].ID != "peer-1" {
		t.Fatalf("listed %+v", listed.Nodes)
	}

	res = doProxyJSON(t, p, http.MethodDelete, "/api/admin/node/peer-1", nil)
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("kick %d: %s", res.StatusCode, b)
	}
	var kicked api.KickPeerResponse
	if err := json.NewDecoder(res.Body).Decode(&kicked); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if kicked.NodeID != "peer-1" {
		t.Fatalf("kicked %+v", kicked)
	}

	res = doProxyJSON(t, p, http.MethodGet, "/api/admin/node", nil)
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()
	if len(listed.Nodes) != 0 {
		t.Fatalf("after kick %+v", listed.Nodes)
	}
}
