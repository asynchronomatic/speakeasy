package security

import (
	"crypto/rand"
	"net/http"
	"strings"

	"github.com/jxskiss/base62"
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

func PasswordHash(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func PasswordCompare(stored, received []byte) error {
	return bcrypt.CompareHashAndPassword(stored, received)
}

func PasswordHashAndEncode(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return base62.EncodeToString(hash), nil
}

func MustPasswordHashAndEncodeBase62(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return base62.EncodeToString(hash)
}

func CompareBase62Password(encoded string, provided string) error {
	decoded, err := base62.DecodeString(encoded)
	if err != nil {
		return err
	}

	return bcrypt.CompareHashAndPassword(decoded, []byte(provided))
}

func DummySecretMatchEx(secret []byte) {
	_ = bcrypt.CompareHashAndPassword(DummyHash, secret)
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

func HashSecret(secret []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
}

func RandomSecret(byteCount int) ([]byte, error) {
	b := make([]byte, byteCount)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
