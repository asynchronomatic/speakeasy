package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

type TokenAuth struct {
	tokens map[string]TokenUser
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

func getToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		token := strings.TrimSpace(auth[len(prefix):])
		return token
	}
	return ""
}

func (a *TokenAuth) DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int) {
	token := getToken(r)
	if token == "" {
		return nil, http.StatusUnauthorized
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
