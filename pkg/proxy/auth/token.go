package auth

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/asynchronomatic/speakeasy/pkg/security"
)

type TokenAuth struct {
	mu       sync.Mutex
	creds    []TokenUser
	verified []verifiedToken
	macKey   []byte
}

type TokenUser struct {
	User         string
	Group        string
	PasswordHash []byte
}

type verifiedToken struct {
	mac  []byte
	user TokenUser
}

func getToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}

func (a *TokenAuth) lookupVerified(mac []byte) *TokenUser {
	for i := range a.verified {
		if security.MACEqual(a.verified[i].mac, mac) {
			u := a.verified[i].user
			return &u
		}
	}
	return nil
}

func (a *TokenAuth) DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int) {
	token := getToken(r)
	if token == "" {
		return nil, http.StatusUnauthorized
	}

	ip := security.ClientHost(r)
	if security.AuthBlocked(ip) {
		return nil, http.StatusTooManyRequests
	}

	mac := security.TokenMAC(a.macKey, token)
	a.mu.Lock()
	if u := a.lookupVerified(mac); u != nil {
		a.mu.Unlock()
		return &Properties{User: u.User, Group: u.Group}, http.StatusOK
	}
	creds := a.creds
	a.mu.Unlock()

	for i := range creds {
		if security.SecretMatch(creds[i].PasswordHash, token) {
			a.mu.Lock()
			a.verified = append(a.verified, verifiedToken{mac: mac, user: creds[i]})
			a.mu.Unlock()
			return &Properties{User: creds[i].User, Group: creds[i].Group}, http.StatusOK
		}
	}

	security.DummySecretMatch(token)
	security.AuthFailure(ip)
	return nil, http.StatusUnauthorized
}

func (a *TokenAuth) AddToken(token string, user string, group string) error {
	user = strings.TrimSpace(user)
	if user == "" || token == "" {
		return errors.New("user and password are required")
	}
	hashed, err := security.HashSecret(token)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.creds = append(a.creds, TokenUser{User: user, Group: group, PasswordHash: hashed})
	a.mu.Unlock()
	return nil
}

func NewTokenAuth() *TokenAuth {
	key, err := security.NewMACKey()
	if err != nil {
		panic(err)
	}
	return &TokenAuth{macKey: key}
}
