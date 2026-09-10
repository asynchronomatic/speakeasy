package auth

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInferenceAuth(t *testing.T) {
	a := NewInferenceAuth()

	hashed, actual, err := a.CreateSecret()
	require.NoError(t, err)
	assert.NotEqual(t, hashed, actual)
	assert.True(t, strings.HasPrefix(hashed, InferenceTokenPrefix))

	fmt.Printf("original: %s\n", actual)
	fmt.Printf("hashed: %s\n", hashed)

	props, code := a.checkToken(actual)
	require.Equal(t, http.StatusOK, code)
	assert.NotNil(t, props)

	props, code = a.checkToken(hashed)
	require.Equal(t, http.StatusUnauthorized, code)
	assert.Nil(t, props)

	key, _, err := ParseInferenceToken(actual)
	require.NoError(t, err)

	err = a.RevokeTokenByKey(key)
	require.NoError(t, err)

	props, code = a.checkToken(actual)
	require.Equal(t, http.StatusUnauthorized, code)
	assert.Nil(t, props)
}
