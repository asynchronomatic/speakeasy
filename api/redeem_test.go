package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/config"
)

func TestRedeemInvite(t *testing.T) {
	var gotPath string
	var gotReq RedeemInviteRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RedeemInviteResponse{
			MeshId:     "default",
			MeshSecret: "secret",
			MeshServer: "https://mesh.example:4002",
		})
	}))
	t.Cleanup(ts.Close)

	token := "abc-invite-token"
	resp, err := RedeemInvite(ts.URL+"/api/v1/redeem/"+token, Node{ID: "peer-1", Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/redeem/"+token {
		t.Fatalf("path %q", gotPath)
	}
	if gotReq.Node.ID != "peer-1" || gotReq.Node.Name != "n1" {
		t.Fatalf("request %+v", gotReq)
	}
	if resp.MeshId != "default" || resp.MeshSecret != "secret" {
		t.Fatalf("response %+v", resp)
	}
	assert.Equal(t, "https://mesh.example:4002", resp.MeshServer)
}

func TestRedeemInviteRequiresNodeID(t *testing.T) {
	_, err := RedeemInvite("http://example:4002/api/v1/redeem/x", Node{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRedeemInviteInvalidURL(t *testing.T) {
	_, err := RedeemInvite("/api/v1/redeem/x", Node{ID: "peer-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseInviteURLMissingSlash(t *testing.T) {
	u, err := parseInviteURL("http:/10.0.0.30:4002/api/v1/redeem/832beddac28840510e73979e378ce5d14d7ede3aa2ea53aa718a445464d6b830")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "http" || u.Host != "10.0.0.30:4002" {
		t.Fatalf("got scheme=%q host=%q", u.Scheme, u.Host)
	}
	if u.Path != "/api/v1/redeem/832beddac28840510e73979e378ce5d14d7ede3aa2ea53aa718a445464d6b830" {
		t.Fatalf("path %q", u.Path)
	}

	u, err = parseInviteURL("https:/example.com:4002/api/v1/redeem/abc")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "example.com:4002" || u.Path != "/api/v1/redeem/abc" {
		t.Fatalf("got %+v", u)
	}
}

func TestRedeemInviteHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invite expired", http.StatusGone)
	}))
	t.Cleanup(ts.Close)

	_, err := RedeemInvite(ts.URL+"/api/v1/redeem/expired", Node{ID: "peer-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "invite expired") {
		t.Fatalf("error leaked body: %v", err)
	}
}

func TestRedeemInviteRejectsMetadata(t *testing.T) {
	_, err := RedeemInvite("http://169.254.169.254/latest/meta-data", Node{ID: "peer-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, config.ErrProviderURLMetadata) {
		t.Fatalf("err=%v want metadata", err)
	}
}

func TestRedeemInviteRejectsUserinfo(t *testing.T) {
	_, err := RedeemInvite("http://user:pass@example.com/api/v1/redeem/x", Node{ID: "peer-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, config.ErrProviderURLUserinfo) {
		t.Fatalf("err=%v want userinfo", err)
	}
}

func TestRedeemInviteDoesNotFollowRedirect(t *testing.T) {
	hit := false
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(dest.Close)

	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL+"/imds", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(src.Close)

	_, err := RedeemInvite(src.URL+"/api/v1/redeem/x", Node{ID: "peer-1"})
	if err == nil {
		t.Fatal("expected redirect error")
	}
	if hit {
		t.Fatal("followed redirect")
	}
}
