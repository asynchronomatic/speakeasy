package security

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var DummyHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("speakeasy-dummy-secret-not-used"), bcrypt.DefaultCost)
	if err != nil {
		panic("security: dummy password hash: " + err.Error())
	}
	DummyHash = h
}

func DummySecretMatch(secret string) {
	_ = bcrypt.CompareHashAndPassword(DummyHash, []byte(secret))
}

func HashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func GetToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		token := strings.TrimSpace(auth[len(prefix):])
		return token
	}
	return ""
}
