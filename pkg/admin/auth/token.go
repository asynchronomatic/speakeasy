package auth

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/jxskiss/base62"
	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/pkg/secrets"
)

const SessionTokenPrefix = "mesh-"

type SessionAuthFunc func(token string) (*Properties, bool)

type TokenAuth struct {
	lock    sync.RWMutex
	tokens  map[string]TokenUser
	session SessionAuthFunc
}

type TokenUser struct {
	User     string
	Group    string
	Password []byte
}

func (a *TokenAuth) SetSessionAuth(fn SessionAuthFunc) {
	a.session = fn
}

func (a *TokenAuth) DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int) {
	token := secrets.GetToken(r)
	if token == "" {
		return nil, http.StatusUnauthorized
	}

	if strings.HasPrefix(token, SessionTokenPrefix) {
		if a.session == nil {
			return nil, http.StatusUnauthorized
		}
		user, ok := a.session(token)
		if !ok || user == nil {
			return nil, http.StatusUnauthorized
		}
		return user, http.StatusOK
	}

	// FIXME: only support one auth user
	// TODO: this neds to be reworked... to follow what we do for proxy
	a.lock.Lock()
	u, ok := a.tokens["admin"]
	a.lock.Unlock()
	if !ok {
		secrets.DummySecretMatch(token)
		return nil, http.StatusUnauthorized
	}

	err := bcrypt.CompareHashAndPassword(u.Password, []byte(token))
	if err != nil {
		return nil, http.StatusUnauthorized
	}

	return &Properties{
		User:  u.User,
		Group: u.Group,
	}, http.StatusOK
}

// AddToken the token maps to a specific user
func (a *TokenAuth) AddToken(token string, user string, group string) error {
	if user == "" || group == "" || token == "" {
		return fmt.Errorf("token, user and group must be set")
	}

	hashed, err := base62.DecodeString(token)
	if err != nil {
		return nil
	}
	// TODO allow multiple users later
	a.tokens["admin"] = TokenUser{
		User:     user,
		Group:    group,
		Password: hashed,
	}
	return nil
}

func NewTokenAuth() *TokenAuth {
	return &TokenAuth{
		tokens: make(map[string]TokenUser),
	}
}
