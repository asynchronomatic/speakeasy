package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

const SessionTokenPrefix = "mesh-"

type SessionAuthFunc func(token string) (*Properties, bool)

type TokenAuth struct {
	tokens  map[string]TokenUser
	session SessionAuthFunc
}

type TokenUser struct {
	User         string
	Group        string
	PasswordHash []byte
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func (a *TokenAuth) SetSessionAuth(fn SessionAuthFunc) {
	a.session = fn
}

func (a *TokenAuth) DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int) {
	auth := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		token := strings.TrimSpace(auth[len(prefix):])

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

		presented := hashPassword(token)
		u, ok := a.tokens[presented]
		if !ok {
			return nil, http.StatusUnauthorized
		}

		return &Properties{
			User:  u.User,
			Group: u.Group,
		}, http.StatusOK
	}
	return nil, http.StatusUnauthorized
}

// AddToken the token maps to a specific user
func (a *TokenAuth) AddToken(token string, user string, group string) error {
	user = strings.TrimSpace(user)
	if user == "" || token == "" {
		return errors.New("user and password are required")
	}

	hashed := hashPassword(token)
	a.tokens[hashed] = TokenUser{
		User:  user,
		Group: group,
	}
	return nil
}

func NewTokenAuth() *TokenAuth {
	return &TokenAuth{
		tokens: make(map[string]TokenUser),
	}
}
