package security

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashSecretAndMatch(t *testing.T) {
	h, err := HashSecret("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if string(h) == "correct-horse" {
		t.Fatal("plaintext stored")
	}
	if !SecretMatch(h, "correct-horse") {
		t.Fatal("expected match")
	}
	if SecretMatch(h, "wrong") {
		t.Fatal("wrong secret matched")
	}
}

func TestTokenMACEqual(t *testing.T) {
	key, err := NewMACKey()
	if err != nil {
		t.Fatal(err)
	}
	a := TokenMAC(key, "tok")
	b := TokenMAC(key, "tok")
	if !MACEqual(a, b) {
		t.Fatal("same token mac mismatch")
	}
	if MACEqual(a, TokenMAC(key, "other")) {
		t.Fatal("different tokens collided")
	}
}

func TestDummyHashIsBcrypt(t *testing.T) {
	if err := bcrypt.CompareHashAndPassword(DummyHash, []byte("nope")); err == nil {
		t.Fatal("dummy hash accepted nope")
	}
}
