package proxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/testable"
)

func TestInferenceTokensListEmpty(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	var listed inferenceTokenList
	err := doProxyJSON(t, p, http.MethodGet, "/api/proxy/inference/tokens", nil, &listed)
	assert.NoError(t, err)
	assert.False(t, listed.Insecure)
	require.NotNil(t, listed.Tokens)
	assert.Empty(t, listed.Tokens)
}

func TestInferenceTokenCreateListDelete(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	// create a token
	var created inferenceToken
	err := doProxyJSON(t, p, http.MethodPost, "/api/proxy/inference/tokens", inferenceTokenCreateRequest{
		Token: &inferenceToken{Name: "ci"},
	}, &created)
	assert.NoError(t, err)
	assert.Equal(t, "ci", created.Name)
	assert.WithinDuration(t, time.Now().UTC(), created.CreatedAt, time.Minute)

	cfg := cm.Config()
	require.Len(t, cfg.Proxy.InferenceTokens.Tokens, 1)
	stored := cfg.Proxy.InferenceTokens.Tokens[0]
	assert.Equal(t, "ci", stored.Name)
	assert.NotEqual(t, created.Secret, stored.Token)

	// list tokens and check their values
	var listed inferenceTokenList
	err = doProxyJSON(t, p, http.MethodGet, "/api/proxy/inference/tokens", nil, &listed)
	assert.NoError(t, err)
	require.Len(t, listed.Tokens, 1)
	assert.Equal(t, "ci", listed.Tokens[0].Name)
	assert.Equal(t, Truncate(stored.Token), listed.Tokens[0].Token)
	assert.Empty(t, listed.Tokens[0].Secret)
	assert.False(t, listed.Insecure)

	_, code := p.inferenceAuth.DoAuth(nil, &http.Request{
		Header: http.Header{
			"Authorization": []string{"Bearer " + created.Secret},
		},
	})
	assert.Equal(t, http.StatusOK, code)

	err = doProxyJSON(t, p, http.MethodDelete, "/api/proxy/inference/tokens/"+listed.Tokens[0].Token, nil, nil)
	assert.NoError(t, err)

	cfg = cm.Config()
	assert.Empty(t, cfg.Proxy.InferenceTokens.Tokens)

	err = doProxyJSON(t, p, http.MethodGet, "/api/proxy/inference/tokens", nil, &listed)
	assert.NoError(t, err)
	assert.Empty(t, listed.Tokens)
	assert.False(t, listed.Insecure)

	_, code = p.inferenceAuth.DoAuth(nil, &http.Request{
		Header: http.Header{
			"Authorization": []string{"Bearer " + created.Secret},
		},
	})
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestInferenceTokenCreateRequiresName(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	err := doProxyJSON(t, p, http.MethodPost, "/api/proxy/inference/tokens", inferenceTokenCreateRequest{
		Token: &inferenceToken{},
	}, nil)
	assert.Equal(t, http.StatusBadRequest, statusCode(err))
}

func TestInferenceTokenCreateDuplicateName(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	err := doProxyJSON(t, p, http.MethodPost, "/api/proxy/inference/tokens", inferenceTokenCreateRequest{
		Token: &inferenceToken{Name: "ci"},
	}, nil)
	assert.NoError(t, err)
	err = doProxyJSON(t, p, http.MethodPost, "/api/proxy/inference/tokens", inferenceTokenCreateRequest{
		Token: &inferenceToken{Name: "ci"},
	}, nil)
	// duplicate names are allowed
	assert.NoError(t, err)
}

func TestInferenceTokenToggleInsecure(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	var listed inferenceTokenList
	err := doProxyJSON(t, p, http.MethodPost, "/api/proxy/inference/tokens", inferenceTokenCreateRequest{
		Insecure: true,
	}, &listed)
	assert.NoError(t, err)
	assert.True(t, listed.Insecure)

	cfg := cm.Config()
	assert.True(t, cfg.Proxy.InferenceTokens.Insecure)
	assert.Empty(t, cfg.Proxy.InferenceTokens.Tokens)

	err = doProxyJSON(t, p, http.MethodGet, "/api/proxy/inference/tokens", nil, &listed)
	assert.NoError(t, err)
	assert.True(t, listed.Insecure)

	err = doProxyJSON(t, p, http.MethodPost, "/api/proxy/inference/tokens", inferenceTokenCreateRequest{
		Insecure: true,
		Token:    &inferenceToken{Name: "ci"},
	}, nil)
	assert.NoError(t, err)

	cfg = cm.Config()
	assert.True(t, cfg.Proxy.InferenceTokens.Insecure)
	require.Len(t, cfg.Proxy.InferenceTokens.Tokens, 1)
}

func TestInferenceTokenDeleteMissing(t *testing.T) {
	cm := testable.MustConfigManager(testConfigYAML)
	p := newTestProxy(t, cm)

	err := doProxyJSON(t, p, http.MethodDelete, "/api/proxy/inference/tokens/nope", nil, nil)
	assert.Equalf(t, http.StatusNotFound, statusCode(err), err.Error())
}
