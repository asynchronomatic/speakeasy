package auth

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/jxskiss/base62"
	"github.com/negrel/assert"

	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

var InferenceTokenPrefix = "se-"
var InvalidInferenceToken = errors.New("invalid inference token")
var InferenceKeyLength = 8
var InferenceSecretLength = 56

type InferenceSecret struct {
	HashedSecret []byte
}

type InferenceAuth struct {
	insecure bool
	tokens   map[string]InferenceSecret
	lock     sync.RWMutex
}

func ParseInferenceToken(authToken string) (string, []byte, error) {
	if !strings.HasPrefix(authToken, InferenceTokenPrefix) {
		return "", nil, InvalidInferenceToken
	}

	parts := strings.Split(strings.TrimPrefix(authToken, InferenceTokenPrefix), "$")
	if len(parts) != 2 {
		return "", nil, InvalidInferenceToken
	}

	key := parts[0]

	secret, err := base62.DecodeString(parts[1])
	if err != nil {
		return "", nil, InvalidInferenceToken
	}

	return key, secret, nil
}

func (a *InferenceAuth) checkToken(token string) (*Properties, int) {
	key, secret, err := ParseInferenceToken(token)
	if err != nil {
		return nil, http.StatusUnauthorized
	}

	a.lock.RLock()
	stored, found := a.tokens[key]
	a.lock.RUnlock()
	log.Printf("checking token %s:%v", key, found)
	if !found {
		security.DummySecretMatchEx(secret)
		return nil, http.StatusUnauthorized
	}

	err = security.PasswordCompare(stored.HashedSecret, secret)
	if err != nil {
		return nil, http.StatusUnauthorized
	}

	return &Properties{
		User:  "inference",
		Group: "inference",
	}, http.StatusOK
}

func (a *InferenceAuth) DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int) {
	if a.insecure {
		return &Properties{
			User:  "inference",
			Group: "inference",
		}, http.StatusOK
	}

	token := getToken(r)
	if token == "" {
		return nil, http.StatusUnauthorized
	}
	return a.checkToken(token)
}

// RevokeToken
func (a *InferenceAuth) RevokeTokenByKey(key string) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	delete(a.tokens, key)
	return nil
}

func (a *InferenceAuth) RevokeToken(token string) error {
	key, _, err := ParseInferenceToken(token)
	if err != nil {
		assert.NoError(err) // we don't ever expect to encounter this
		return err
	}

	a.lock.Lock()
	defer a.lock.Unlock()
	if _, ok := a.tokens[key]; !ok {
		assert.FailNow("token not found")
		return nil
	}

	delete(a.tokens, key)
	return nil
}

// AddToken the token maps to a specific user
func (a *InferenceAuth) AddToken(token string) error {
	key, hash, err := ParseInferenceToken(token)
	if err != nil {
		return err
	}

	a.lock.Lock()
	a.tokens[key] = InferenceSecret{
		HashedSecret: hash,
	}
	a.lock.Unlock()
	return nil
}

func (a *InferenceAuth) CreateSecret() (string, string, error) {
	for {
		key, err := security.RandomSecret(InferenceKeyLength)
		if err != nil {
			return "", "", err
		}

		secretKey := base62.EncodeToString(key)
		if _, ok := a.tokens[secretKey]; ok {
			continue // retry somehow we got a duplicate
		}

		secret, err := security.RandomSecret(InferenceSecretLength)
		if err != nil {
			return "", "", err
		}

		secretHash, err := security.HashSecret(secret)
		if err != nil {
			return "", "", err
		}

		theirValue := InferenceTokenPrefix + secretKey + "$" + base62.EncodeToString(secret)
		outValue := InferenceTokenPrefix + secretKey + "$" + base62.EncodeToString(secretHash)

		a.lock.Lock()
		_, ok := a.tokens[secretKey]
		if !ok {
			a.tokens[secretKey] = InferenceSecret{
				HashedSecret: secretHash,
			}
		}
		a.lock.Unlock()
		if ok {
			continue
		}
		return outValue, theirValue, nil
	}
}

func (a *InferenceAuth) SetInsecure(insecure bool) {
	a.insecure = insecure
}

func NewInferenceAuth() *InferenceAuth {
	return &InferenceAuth{
		insecure: false,
		tokens:   make(map[string]InferenceSecret),
	}
}
