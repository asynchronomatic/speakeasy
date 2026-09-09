package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// DummyHash is a real bcrypt hash so failed password checks can pay the same cost.
var DummyHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("speakeasy-dummy-secret-not-used"), bcrypt.DefaultCost)
	if err != nil {
		panic("security: dummy password hash: " + err.Error())
	}
	DummyHash = h
}

func HashSecret(secret string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
}

func SecretMatch(hash []byte, secret string) bool {
	if len(hash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(secret)) == nil
}

func DummySecretMatch(secret string) {
	_ = bcrypt.CompareHashAndPassword(DummyHash, []byte(secret))
}

func NewMACKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("mac key: %w", err)
	}
	return key, nil
}

func TokenMAC(key []byte, token string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(token))
	return m.Sum(nil)
}

func MACEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}
