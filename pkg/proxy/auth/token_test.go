package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func bearerReq(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestAddTokenHashesPassword(t *testing.T) {
	a := NewTokenAuth()
	if err := a.AddToken("secret", "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	if len(a.creds) != 1 || string(a.creds[0].PasswordHash) == "secret" {
		t.Fatal("plaintext password stored")
	}
}

func TestDoAuthAcceptsPassword(t *testing.T) {
	a := NewTokenAuth()
	if err := a.AddToken("secret", "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	got, code := a.DoAuth(nil, bearerReq("secret"))
	if code != http.StatusOK || got == nil || got.User != "admin" || got.Group != "admin" {
		t.Fatalf("got %+v code %d", got, code)
	}
}

func TestDoAuthRejectsWrongPassword(t *testing.T) {
	a := NewTokenAuth()
	if err := a.AddToken("secret", "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	got, code := a.DoAuth(nil, bearerReq("wrong"))
	if code != http.StatusUnauthorized || got != nil {
		t.Fatalf("got %+v code %d", got, code)
	}
}

func TestDoAuthRejectsEmptyBearer(t *testing.T) {
	a := NewTokenAuth()
	_ = a.AddToken("secret", "admin", "admin")
	if _, code := a.DoAuth(nil, httptest.NewRequest(http.MethodGet, "/x", nil)); code != http.StatusUnauthorized {
		t.Fatalf("code %d", code)
	}
}

func TestAddTokenRequiresFields(t *testing.T) {
	a := NewTokenAuth()
	if err := a.AddToken("secret", "", "admin"); err == nil {
		t.Fatal("expected error")
	}
	if err := a.AddToken("", "admin", "admin"); err == nil {
		t.Fatal("expected error")
	}
}
