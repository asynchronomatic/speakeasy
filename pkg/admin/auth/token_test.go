package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func bearerReq(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestAddUserHashesPassword(t *testing.T) {
	a := NewTokenAuth()
	if err := a.AddToken("secret", "admin", "admin"); err != nil {
		t.Fatal(err)
	}

	if _, exists := a.tokens["secret"]; exists {
		t.Fatal("plaintext password used as map key")
	}
}

func TestDoAuthAcceptsPassword(t *testing.T) {
	a := NewTokenAuth()

	err := a.AddToken(security.MustPasswordHashAndEncodeBase62("secret"), "admin", "admin")
	assert.NoError(t, err)
	got, code := a.DoAuth(nil, bearerReq("secret"))
	assert.Equal(t, http.StatusOK, code)
	require.NotNil(t, got)
	assert.Equal(t, "admin", got.User)
	assert.Equal(t, "admin", got.Group)
}

func TestDoAuthRejectsWrongPassword(t *testing.T) {
	a := NewTokenAuth()
	err := a.AddToken(security.MustPasswordHashAndEncodeBase62("secret"), "admin", "admin")
	assert.NoError(t, err)
	got, code := a.DoAuth(nil, bearerReq("wrong"))
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Nil(t, got)
}

func TestDoAuthRejectsEmptyBearer(t *testing.T) {
	a := NewTokenAuth()
	err := a.AddToken(security.MustPasswordHashAndEncodeBase62("secret"), "admin", "admin")
	assert.NoError(t, err)
	_, code := a.DoAuth(nil, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestAddUserRequiresFields(t *testing.T) {
	var err error
	a := NewTokenAuth()
	err = a.AddToken("secret", "", "admin")
	assert.Error(t, err)
	err = a.AddToken("", "admin", "admin")
	assert.Error(t, err)
}
