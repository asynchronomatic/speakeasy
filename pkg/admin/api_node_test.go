package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
	"github.com/asynchronomatic/speakeasy/pkg/admin/magiclink"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
)

func TestApiNodeLogin(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest"})
	join, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-login-1", Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-login-1",
		MeshId:     "default",
		MeshSecret: join.MeshSecret,
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: got %d", res.StatusCode)
	}
	var login api.NodeLoginResponse
	if err := json.NewDecoder(res.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	if login.NodeID != "peer-login-1" {
		t.Fatalf("NodeID=%q", login.NodeID)
	}
	if !strings.HasPrefix(login.Token, auth.SessionTokenPrefix) {
		t.Fatalf("token prefix: %q", login.Token)
	}
	now := time.Now().Unix()
	if login.Expires <= now {
		t.Fatalf("expected future expiry, got %d", login.Expires)
	}
	maxExp := time.Now().Add(SessionTokenTTL + time.Second).Unix()
	if login.Expires > maxExp {
		t.Fatalf("expiry %d beyond TTL (max %d)", login.Expires, maxExp)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	authRes, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	authRes.Body.Close()
	if authRes.StatusCode != http.StatusOK {
		t.Fatalf("session GET /nodes: got %d", authRes.StatusCode)
	}

	props, err := s.authenticateSessionToken(login.Token)
	if err != nil {
		t.Fatal(err)
	}
	if props.User != "peer-login-1" || props.Group != MeshGroup {
		t.Fatalf("session props %+v", props)
	}
}

func TestImmortalSessionRejected(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-immortal", Name: "n1"}); err != nil {
		t.Fatal(err)
	}

	claims := sessionClaims{NodeID: "peer-immortal", Expires: 0}
	raw, err := magiclink.New(s.magicKey).Encrypt(&claims)
	if err != nil {
		t.Fatal(err)
	}
	token := auth.SessionTokenPrefix + raw

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("immortal session: got %d want 401", res.StatusCode)
	}
}

func TestSessionRejectedAfterACLRemove(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest"})
	join, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-acl-1", Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-acl-1",
		MeshId:     "default",
		MeshSecret: join.MeshSecret,
	})
	var login api.NodeLoginResponse
	if err := json.NewDecoder(res.Body).Decode(&login); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()

	s.acl.Remove("peer-acl-1")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	authRes, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	authRes.Body.Close()
	if authRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("kicked session: got %d want 401", authRes.StatusCode)
	}

	authz := postJSON(t, ts, http.MethodPost, "/api/v1/authorize", login.Token, api.RegisterNodeRequest{
		Node: api.Node{ID: "peer-acl-1", Name: "n1"},
	})
	authz.Body.Close()
	if authz.StatusCode != http.StatusUnauthorized {
		t.Fatalf("kicked authorize: got %d want 401", authz.StatusCode)
	}
}

func TestSessionRejectedAfterKick(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Name: "guest"})
	join, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-kick-session", Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-kick-session",
		MeshSecret: join.MeshSecret,
	})
	var login api.NodeLoginResponse
	if err := json.NewDecoder(res.Body).Decode(&login); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()

	kick := postJSON(t, ts, http.MethodDelete, "/api/v1/admin/nodes/peer-kick-session", "test-secret", nil)
	kick.Body.Close()
	if kick.StatusCode != http.StatusOK {
		t.Fatalf("kick: got %d", kick.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	authRes, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	authRes.Body.Close()
	if authRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session after kick: got %d want 401", authRes.StatusCode)
	}

	relogin := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-kick-session",
		MeshSecret: join.MeshSecret,
	})
	relogin.Body.Close()
	if relogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login after kick: got %d want 401", relogin.StatusCode)
	}
}

func TestApiNodeLoginRejectsBadSecret(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-login-bad", Name: "n1"}); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-login-bad",
		MeshSecret: "wrong-secret",
	})
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad secret: got %d want 401", res.StatusCode)
	}
}

func TestApiNodeLoginExpiredSession(t *testing.T) {
	s, ts := newAdminTestServer(t)
	claims := sessionClaims{NodeID: "peer-expired-session", Expires: time.Now().Add(-time.Second).Unix()}
	raw, err := magiclink.New(s.magicKey).Encrypt(&claims)
	if err != nil {
		t.Fatal(err)
	}
	token := auth.SessionTokenPrefix + raw

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired session: got %d want 401", res.StatusCode)
	}
}

func TestMeshClientLogin(t *testing.T) {
	_, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1", Reusable: true})
	join, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-client-login", Name: "n1"})
	assert.NoError(t, err)

	_, err = api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-client-bad", Name: "bad"})
	assert.NoError(t, err)

	mc, err := api.NewClient(ts.URL, "").Mesh("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := mc.Login("peer-client-login", join.MeshSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := mc.GetPeers(); err != nil {
		t.Fatalf("session after login: %v", err)
	}

	_, err = mc.Register("n1", "peer-client-login")
	assert.NoError(t, err)

	_, err = mc.Register("n1", "peer-client-bad")
	assert.Error(t, err)

	err = mc.Unregister("peer-client-bad")
	assert.Error(t, err)

	err = mc.Unregister("peer-client-login")
	assert.NoError(t, err)
}

func TestUnregisterDeletesCredentials(t *testing.T) {
	s, ts := newAdminTestServer(t)
	created := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	join, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+created.InviteId, api.Node{ID: "peer-unreg", Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-unreg",
		MeshSecret: join.MeshSecret,
	})
	var login api.NodeLoginResponse
	if err := json.NewDecoder(res.Body).Decode(&login); err != nil {
		res.Body.Close()
		t.Fatal(err)
	}
	res.Body.Close()

	reg := postJSON(t, ts, http.MethodPost, "/api/v1/nodes", login.Token, api.RegisterNodeRequest{
		Node: api.Node{ID: "peer-unreg", Name: "n1"},
	})
	reg.Body.Close()
	if reg.StatusCode != http.StatusOK {
		t.Fatalf("register: got %d", reg.StatusCode)
	}

	unreg := postJSON(t, ts, http.MethodDelete, "/api/v1/nodes/peer-unreg", login.Token, nil)
	unreg.Body.Close()
	if unreg.StatusCode != http.StatusOK {
		t.Fatalf("unregister: got %d", unreg.StatusCode)
	}

	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey("default", "peer-unreg"), &rec); !errors.Is(err, jsonkv.ErrNotFound) {
		t.Fatalf("kv record still stored: %v", err)
	}
	if s.acl.Has("peer-unreg") {
		t.Fatal("node still on ACL")
	}

	relogin := postJSON(t, ts, http.MethodPost, "/api/v1/login", "", api.NodeLoginRequest{
		NodeID:     "peer-unreg",
		MeshSecret: join.MeshSecret,
	})
	relogin.Body.Close()
	if relogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login after unregister: got %d want 401", relogin.StatusCode)
	}

	fresh := createInvite(t, ts, api.CreateInviteRequest{MeshId: "mesh-1"})
	if _, err := api.RedeemInvite(ts.URL+"/api/v1/redeem/"+fresh.InviteId, api.Node{ID: "peer-unreg", Name: "n1"}); err != nil {
		t.Fatalf("rejoin after unregister: %v", err)
	}
}
