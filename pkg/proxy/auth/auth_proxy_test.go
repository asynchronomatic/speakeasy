package auth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAuth(t *testing.T) {
	a := NewUserAuth()
	u := a.WithUser("admin", "test", "test-password")
	if u == nil {
		t.Errorf("expected user to be created")
	}

	token, code := a.LoginApi("admin", "test-password")
	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, token)

	req, err := http.NewRequest("GET", "/test", nil)
	assert.NoError(t, err)
	assert.NotNil(t, req)
	req.Header.Set("Authorization", "Bearer "+token)

	p, code := a.DoAuth(nil, req)
	assert.Equal(t, http.StatusOK, code)
	assert.NotNil(t, p)

	_, code = a.LoginApi("admin", "wrong")
	assert.Equal(t, http.StatusUnauthorized, code)
}
