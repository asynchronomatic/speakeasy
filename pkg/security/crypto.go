package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

type EncryptionKey []byte

func (e EncryptionKey) String() string {
	return hex.EncodeToString(e)
}

const keySize = 32 // AES-256

func GenerateKey() (EncryptionKey, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func Encrypt(plaintext string, key EncryptionKey) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// nonce || ciphertext || tag
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func Decrypt(ciphertextB64 string, key EncryptionKey) (string, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}
	return string(plain), nil
}

func EncryptObject(v any, key EncryptionKey) (string, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return Encrypt(string(jsonBytes), key)
}

func DecryptObject(value string, v any, key EncryptionKey) error {
	data, err := Decrypt(value, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), v)
}
