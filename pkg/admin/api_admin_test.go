package admin

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
)

// testHTTPClient disables keep-alives so sequential httptest servers on macOS
// do not exhaust loopback ephemeral ports (EADDRNOTAVAIL / "can't assign requested address").
var testHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DisableKeepAlives: true,
	},
}

func closeIdleHTTP() {
	testHTTPClient.CloseIdleConnections()
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
}

func newAdminTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	t.Setenv("ADMIN_DB_PATH", filepath.Join(t.TempDir(), "admin.jkv"))
	s, err := NewServer(":0", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.WithAdvertiseURL("https://mesh.example:4002")
	s.WithRelayAddresses([]string{"/ip4/1.2.3.4/tcp/4001/p2p/relay"})
	ts := httptest.NewUnstartedServer(s.routes())
	ts.Config.SetKeepAlivesEnabled(false)
	ts.Start()
	t.Cleanup(func() {
		ts.Close()
		closeIdleHTTP()
	})
	return s, ts
}

func postJSON(t *testing.T, ts *httptest.Server, method, path, token string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func createInvite(t *testing.T, ts *httptest.Server, req api.CreateInviteRequest) api.CreateInviteResponse {
	t.Helper()
	res := postJSON(t, ts, http.MethodPost, "/api/v1/admin/invite", "test-secret", req)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create invite: got %d", res.StatusCode)
	}
	var resp api.CreateInviteResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.InviteId == "" {
		t.Fatal("expected invite id")
	}
	if len(resp.InviteId) != 64 {
		t.Fatalf("invite id length %d, want 64-char sha256 hex: %q", len(resp.InviteId), resp.InviteId)
	}
	if _, err := hex.DecodeString(resp.InviteId); err != nil {
		t.Fatalf("invite id is not hex: %q (%v)", resp.InviteId, err)
	}
	return resp
}

func decodeRedeem(t *testing.T, res *http.Response) api.RedeemInviteResponse {
	t.Helper()
	defer res.Body.Close()
	var resp api.RedeemInviteResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAdminCreateInviteLink(t *testing.T) {
	s, ts := newAdminTestServer(t)

	resp := createInvite(t, ts, api.CreateInviteRequest{
		MeshId:      "mesh-1",
		Name:        "guest",
		Reusable:    false,
		LifetimeSec: 3600,
	})
	wantLink := "https://mesh.example:4002/api/v1/redeem/" + resp.InviteId
	if resp.InviteLink != wantLink {
		t.Fatalf("InviteLink=%q want %q", resp.InviteLink, wantLink)
	}

	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", resp.InviteId), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.UUID == "" || stored.MeshId != "default" || stored.Reusable || stored.InviteAs != "guest" {
		t.Fatalf("stored %+v", stored)
	}
	if stored.Expires <= time.Now().Unix() {
		t.Fatalf("expires %d should be in the future", stored.Expires)
	}
}

func TestAdminCreateInviteLinkDefaults(t *testing.T) {
	s, ts := newAdminTestServer(t)

	resp := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", LifetimeSec: api.DefaultInviteLifetimeSec})
	if resp.Reusable {
		t.Fatal("default invite should be one-time")
	}
	wantExp := time.Now().Add(24 * time.Hour).Unix()
	if resp.Expires < wantExp-5 || resp.Expires > wantExp+5 {
		t.Fatalf("expires %d want ~%d", resp.Expires, wantExp)
	}
	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", resp.InviteId), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Reusable || stored.Expires != resp.Expires {
		t.Fatalf("default invite %+v", stored)
	}
}

func TestAdminCreateInviteLinkForever(t *testing.T) {
	s, ts := newAdminTestServer(t)

	resp := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Reusable: true})
	if !resp.Reusable || resp.Expires != 0 {
		t.Fatalf("forever response %+v", resp)
	}
	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", resp.InviteId), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Expires != 0 || !stored.Reusable {
		t.Fatalf("forever invite %+v", stored)
	}
	if !strings.HasSuffix(resp.InviteLink, "/api/v1/redeem/"+resp.InviteId) {
		t.Fatalf("InviteLink=%q", resp.InviteLink)
	}
}

func TestAdminCreateInviteLinkRequiresMeshID(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodPost, "/api/v1/admin/invite", "test-secret", api.CreateInviteRequest{})
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing mesh id: got %d", res.StatusCode)
	}
}

func TestAdminMutatingJSONRequiresJSONContentType(t *testing.T) {
	_, ts := newAdminTestServer(t)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/invite", strings.NewReader(`{"MeshId":"default"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("Content-Type", "text/plain")
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d want 415", res.StatusCode)
	}
}

func TestAdminMutatingRejectsCrossOrigin(t *testing.T) {
	_, ts := newAdminTestServer(t)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/invite", strings.NewReader(`{"MeshId":"default"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d want 403", res.StatusCode)
	}
}

func TestAdminRedeemInviteLink(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest", Reusable: true})

	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-redeem-1"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("redeem: got %d", res.StatusCode)
	}
	resp := decodeRedeem(t, res)
	if resp.MeshId != "default" {
		t.Fatalf("MeshId=%q", resp.MeshId)
	}
	if resp.MeshSecret == "" || resp.MeshSecret == "test-secret" {
		t.Fatalf("MeshSecret=%q", resp.MeshSecret)
	}
	assert.Equal(t, "https://mesh.example:4002", resp.MeshServer)

	if !s.acl.Has("peer-redeem-1") {
		t.Fatal("expected node to be authorized")
	}

	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-redeem-1"), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.NodeID != "peer-redeem-1" || rec.Name != "guest" || rec.InvitedAs != "guest" {
		t.Fatalf("stored node %+v", rec)
	}
	if rec.AddedAt.IsZero() {
		t.Fatal("expected AddedAt")
	}
	if rec.PasswordHash == "" || rec.PasswordHash == resp.MeshSecret {
		t.Fatalf("expected hashed password, got %q", rec.PasswordHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(resp.MeshSecret)); err != nil {
		t.Fatalf("password hash does not match returned secret: %v", err)
	}

	res = postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-redeem-2"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reuse forever invite: got %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestAdminRedeemRejectsExistingNode(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest", Reusable: true})

	first := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-taken", Name: "n1"},
	})
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first redeem: got %d", first.StatusCode)
	}
	orig := decodeRedeem(t, first)

	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-taken"), &rec); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-taken", Name: "attacker"},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("overwrite redeem: got %d want 409", res.StatusCode)
	}

	var after meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-taken"), &after); err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != rec.PasswordHash || after.Name != "n1" {
		t.Fatalf("credentials changed: %+v -> %+v", rec, after)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte(orig.MeshSecret)); err != nil {
		t.Fatal("original mesh secret no longer valid")
	}

	login := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-taken",
		MeshSecret: orig.MeshSecret,
	})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login with original secret: got %d", login.StatusCode)
	}
}

func TestAdminRedeemConflictDoesNotConsumeInvite(t *testing.T) {
	s, ts := newAdminTestServer(t)
	taken := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+taken.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-exists", Name: "n1"},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("seed redeem: got %d", res.StatusCode)
	}

	fresh := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Reusable: false})
	conflict := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+fresh.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-exists", Name: "n2"},
	})
	conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflict: got %d want 409", conflict.StatusCode)
	}

	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", fresh.InviteId), &stored); err != nil {
		t.Fatalf("one-time invite consumed on conflict: %v", err)
	}

	ok := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+fresh.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-new", Name: "n2"},
	})
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("redeem unused id: got %d", ok.StatusCode)
	}
}

func TestAdminRedeemOneTimeInvite(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Reusable: false})

	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-once-1", Name: "n1"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("first redeem: got %d", res.StatusCode)
	}
	res.Body.Close()
	if !s.acl.Has("peer-once-1") {
		t.Fatal("expected node to be authorized")
	}
	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-once-1"), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.NodeID != "peer-once-1" || rec.Name != "n1" || rec.InvitedAs != "" {
		t.Fatalf("stored node %+v", rec)
	}
	if rec.PasswordHash == "" {
		t.Fatal("expected password hash")
	}

	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", created.InviteId), &stored); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("one-time invite still stored: %v %+v", err, stored)
	}

	res = postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-once-2", Name: "n2"},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("second redeem: got %d want 404", res.StatusCode)
	}
	if s.acl.Has("peer-once-2") {
		t.Fatal("second redeem should not authorize")
	}
}

func TestAdminRedeemExpiredInvite(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", LifetimeSec: 3600})

	var stored inviteSecret
	key := inviteKVKey("default", created.InviteId)
	if err := s.kv.Get(key, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Expires = time.Now().Add(-time.Second).Unix()
	if err := s.kv.Put(key, stored); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-expired-1", Name: "n1"},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusGone {
		t.Fatalf("expired redeem: got %d want 410", res.StatusCode)
	}
	if s.acl.Has("peer-expired-1") {
		t.Fatal("expired invite should not authorize")
	}
	if err := s.kv.Get(key, &stored); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("expired invite still stored: %v", err)
	}
}

func TestAdminRedeemInvalidInvite(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/not-a-valid-token", "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-1"},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("invalid invite: got %d want 404", res.StatusCode)
	}
}

func TestRedeemInviteClient(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})

	resp, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-client-1", Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.MeshId != "default" || resp.MeshSecret == "" || resp.MeshSecret == "test-secret" {
		t.Fatalf("response %+v", resp)
	}
	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-client-1"), &rec); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(resp.MeshSecret)); err != nil {
		t.Fatalf("client redeem hash mismatch: %v", err)
	}
	if !s.acl.Has("peer-client-1") {
		t.Fatal("expected node to be authorized")
	}
}

func TestAdminListInviteLinks(t *testing.T) {
	_, ts := newAdminTestServer(t)
	a := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest", Reusable: false, LifetimeSec: api.DefaultInviteLifetimeSec})
	b := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Reusable: true})

	res := postJSON(t, ts, http.MethodGet, "/api/v1/admin/invite", "test-secret", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list invites: got %d", res.StatusCode)
	}
	var listed api.ListInvitesResponse
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Invites) != 2 {
		t.Fatalf("listed %d, want 2: %+v", len(listed.Invites), listed.Invites)
	}
	byID := map[string]api.InviteInfo{}
	for _, inv := range listed.Invites {
		byID[inv.InviteId] = inv
	}
	ga := byID[a.InviteId]
	if ga.InviteLink != a.InviteLink || ga.Name != "guest" || ga.Reusable {
		t.Fatalf("invite a %+v", ga)
	}
	gb := byID[b.InviteId]
	if gb.InviteLink != b.InviteLink || !gb.Reusable || gb.Expires != 0 {
		t.Fatalf("invite b %+v", gb)
	}
}

func TestAdminListInviteLinksSkipsExpired(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", LifetimeSec: 3600})
	key := inviteKVKey("default", created.InviteId)
	var stored inviteSecret
	if err := s.kv.Get(key, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Expires = time.Now().Add(-time.Minute).Unix()
	if err := s.kv.Put(key, stored); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodGet, "/api/v1/admin/invite", "test-secret", nil)
	defer res.Body.Close()
	var listed api.ListInvitesResponse
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Invites) != 0 {
		t.Fatalf("expired still listed: %+v", listed.Invites)
	}
	if err := s.kv.Get(key, &stored); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("expired invite still stored: %v", err)
	}
}

func TestAdminListInviteLinksRequiresAuth(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodGet, "/api/v1/admin/invite", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: got %d want 401", res.StatusCode)
	}
}

func TestAdminListInviteClient(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "n1"})
	listed, err := api.NewClient(ts.URL, "test-secret").Admin().ListInvites()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Invites) != 1 || listed.Invites[0].InviteId != created.InviteId {
		t.Fatalf("listed %+v", listed.Invites)
	}
}

func TestAdminKickPeer(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-kick-1", Name: "n1"}); err != nil {
		t.Fatal(err)
	}

	reg := postJSON(t, ts, http.MethodPost, "/api/v1/nodes", "test-secret", api.RegisterNodeRequest{
		Node: api.Node{ID: "peer-kick-1", Name: "n1"},
	})
	reg.Body.Close()
	if reg.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d", reg.StatusCode)
	}

	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/peer/peer-kick-1", "test-secret", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("kick: got %d", res.StatusCode)
	}
	var kicked api.KickPeerResponse
	if err := json.NewDecoder(res.Body).Decode(&kicked); err != nil {
		t.Fatal(err)
	}
	if kicked.NodeID != "peer-kick-1" {
		t.Fatalf("kicked %+v", kicked)
	}

	s.lock.Lock()
	_, still := s.nodes["peer-kick-1"]
	s.lock.Unlock()
	if still {
		t.Fatal("node still registered")
	}
	if s.acl.Has("peer-kick-1") {
		t.Fatal("node still on ACL")
	}
	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-kick-1"), &rec); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("kv record still stored: %v", err)
	}
}

func TestAdminKickPeerNotFound(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/peer/missing", "test-secret", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("kick missing: got %d want 404", res.StatusCode)
	}
}

func TestAdminKickPeerRequiresAdmin(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/peer/peer-1", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated kick: got %d want 401", res.StatusCode)
	}
}

func TestAdminListNodes(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-list-1", Name: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := s.kv.Put(meshNodeKVKey("other", "peer-other"), meshNodeRecord{
		NodeID:    "peer-other",
		Name:      "beta",
		AddedAt:   time.Now().UTC(),
		InvitedAs: "other-guest",
	}); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodGet, "/api/v1/admin/nodes", "test-secret", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list nodes: got %d", res.StatusCode)
	}
	var listed api.ListAdminNodesResponse
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Nodes) != 2 {
		t.Fatalf("listed %d, want 2: %+v", len(listed.Nodes), listed.Nodes)
	}
	byID := map[string]api.AdminNode{}
	for _, n := range listed.Nodes {
		byID[n.ID] = n
	}
	a := byID["peer-list-1"]
	if a.Name != "alpha" || a.MeshId != "default" || a.InvitedAs != "guest" || a.AddedAt.IsZero() {
		t.Fatalf("default node %+v", a)
	}
	b := byID["peer-other"]
	if b.Name != "beta" || b.MeshId != "other" || b.InvitedAs != "other-guest" {
		t.Fatalf("other mesh node %+v", b)
	}
}

func TestAdminDeleteNodeAllMeshes(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-del-1", Name: "n1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.kv.Put(meshNodeKVKey("other", "peer-del-1"), meshNodeRecord{
		NodeID:    "peer-del-1",
		Name:      "n1-other",
		AddedAt:   time.Now().UTC(),
		InvitedAs: "guest",
	}); err != nil {
		t.Fatal(err)
	}
	reg := postJSON(t, ts, http.MethodPost, "/api/v1/nodes", "test-secret", api.RegisterNodeRequest{
		Node: api.Node{ID: "peer-del-1", Name: "n1"},
	})
	reg.Body.Close()
	if reg.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d", reg.StatusCode)
	}

	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/nodes/peer-del-1", "test-secret", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete: got %d", res.StatusCode)
	}
	var deleted api.DeleteNodeResponse
	if err := json.NewDecoder(res.Body).Decode(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.NodeID != "peer-del-1" {
		t.Fatalf("deleted %+v", deleted)
	}

	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-del-1"), &rec); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("default mesh record still stored: %v", err)
	}
	if err := s.kv.Get(meshNodeKVKey("other", "peer-del-1"), &rec); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("other mesh record still stored: %v", err)
	}
	if s.acl.Has("peer-del-1") {
		t.Fatal("node still on ACL")
	}
	s.lock.Lock()
	_, still := s.nodes["peer-del-1"]
	s.lock.Unlock()
	if still {
		t.Fatal("node still registered")
	}
}

func TestAdminDeleteNodeNotFound(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/nodes/missing", "test-secret", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing: got %d want 404", res.StatusCode)
	}
}

func TestAdminDeleteNodeRequiresAuth(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/nodes/peer-1", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete: got %d want 401", res.StatusCode)
	}
}

func TestAdminDeleteNodeClient(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-del-client", Name: "n1"}); err != nil {
		t.Fatal(err)
	}
	if err := api.NewClient(ts.URL, "test-secret").Admin().DeleteNode("peer-del-client"); err != nil {
		t.Fatal(err)
	}
	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-del-client"), &rec); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("record still stored: %v", err)
	}
	if s.acl.Has("peer-del-client") {
		t.Fatal("still on ACL")
	}
}

func TestAdminListNodesEmpty(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodGet, "/api/v1/admin/nodes", "test-secret", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("empty list: got %d", res.StatusCode)
	}
	var listed api.ListAdminNodesResponse
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if listed.Nodes == nil || len(listed.Nodes) != 0 {
		t.Fatalf("want empty slice, got %+v", listed.Nodes)
	}
}

func TestAdminListNodesRequiresAuth(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodGet, "/api/v1/admin/nodes", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: got %d want 401", res.StatusCode)
	}
}

func TestAdminListNodesClient(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-list-client", Name: "n1"}); err != nil {
		t.Fatal(err)
	}

	listed, err := api.NewClient(ts.URL, "test-secret").Admin().ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Nodes) != 1 || listed.Nodes[0].ID != "peer-list-client" || listed.Nodes[0].MeshId != "default" {
		t.Fatalf("listed %+v", listed.Nodes)
	}
}

func TestAdminKickPeerClient(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-kick-client", Name: "n1"}); err != nil {
		t.Fatal(err)
	}

	client := api.NewClient(ts.URL, "test-secret")
	if err := client.Admin().KickPeer("peer-kick-client"); err != nil {
		t.Fatal(err)
	}
	if s.acl.Has("peer-kick-client") {
		t.Fatal("still on ACL")
	}
}

func TestAdminDeleteInviteLink(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})

	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/invite/"+created.InviteId, "test-secret", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete invite: got %d", res.StatusCode)
	}

	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", created.InviteId), &stored); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("invite still stored: %v %+v", err, stored)
	}

	res2 := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{
		Node: api.Node{ID: "peer-deleted-1"},
	})
	res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("redeem after delete: got %d want 404", res2.StatusCode)
	}
}

func TestAdminDeleteInviteLinkIdempotent(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})

	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/invite/"+created.InviteId, "test-secret", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("first delete: got %d", res.StatusCode)
	}

	res = postJSON(t, ts, http.MethodDelete, "/api/v1/admin/invite/"+created.InviteId, "test-secret", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: got %d want 404", res.StatusCode)
	}
}

func TestAdminDeleteInviteLinkInvalid(t *testing.T) {
	_, ts := newAdminTestServer(t)
	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/invite/not-a-valid-token", "test-secret", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("invalid delete: got %d want 404", res.StatusCode)
	}
}

func TestAdminDeleteInviteLinkRequiresAuth(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	res := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/invite/"+created.InviteId, "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete: got %d want 401", res.StatusCode)
	}
}

func TestAdminDeleteInviteClient(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})

	client := api.NewClient(ts.URL, "test-secret")
	if err := client.Admin().DeleteInvite(created.InviteId); err != nil {
		t.Fatal(err)
	}

	var stored inviteSecret
	if err := s.kv.Get(inviteKVKey("default", created.InviteId), &stored); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("invite still stored: %v", err)
	}
}

func TestAdminRedeemRequiresNodeID(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	res := postJSON(t, ts, http.MethodPost, "/api/v1/redeem/"+created.InviteId, "", api.RedeemInviteRequest{})
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing node id: got %d want 400", res.StatusCode)
	}
}
