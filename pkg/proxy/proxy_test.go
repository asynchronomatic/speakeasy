package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/testable"
)

var testProviders = []core.Provider{
	{
		ID:        "test-provider-1",
		Type:      "test",
		BaseURL:   "http://test-1",
		Token:     "12345",
		Private:   false,
		Discovery: "whitelist",
		Models: []core.ModelConfig{
			{
				Model:   "test-model-0",
				Private: false,
				Capabilities: []string{
					"embedding", "text",
				},
				Tools: []string{
					"web_search",
				},
			},
			{
				Model:   "test-model-1",
				Private: true,
				Capabilities: []string{
					"text", "image",
				},
				Tools: []string{
					"web_search",
				},
			},
		},
	},
	{
		ID:        "test-provider-2",
		Type:      "test",
		BaseURL:   "http://test-2",
		Token:     "5678",
		Private:   false,
		Discovery: "whitelist",
		Models: []core.ModelConfig{
			{
				Model:   "test-model-0",
				Private: false,
				Capabilities: []string{
					"embedding", "text",
				},
				Tools: []string{
					"web_search",
				},
			},
			{
				Model:   "test-model-1",
				Private: false,
				Capabilities: []string{
					"text", "image",
				},
				Tools: []string{
					"web_search",
				},
			},
		},
	},
}

func TestNewProxy(t *testing.T) {
	orch := testable.NewMeshOrchestrator()

	testMeshLeft := orch.NewMeshNode("000001", "left")
	testMeshRight := orch.NewMeshNode("000002", "right")

	proxyLeft, err := NewProxy(testMeshLeft, ":0", nil, true)
	assert.Nil(t, err)
	assert.NotNil(t, proxyLeft)

	proxyRight, err := NewProxy(testMeshRight, ":0", testProviders, true)
	assert.Nil(t, err)
	assert.NotNil(t, proxyLeft)

	go func() {
		_ = core.RunInterruptibleContext(context.Background(), proxyLeft, proxyRight)
	}()

	client := NewProxyClient("proxy.left", &testable.Doer{
		Handler: proxyLeft.ServeHTTP,
	})

	// Wait for both nodes to come online
	retries := 10
	for {
		members, err := client.GetMeshMembers()
		assert.NoError(t, err)

		if len(members) == 2 {
			break
		}

		time.Sleep(time.Second)
		retries--
		if retries == 0 {
			break
		}
	}
	assert.NotEqual(t, 0, retries)

}
