package proxy

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/testable"
)

func statusCode(err error) int {
	var je *jsonrpc.Error
	if errors.As(err, &je) {
		return je.Code()
	}
	return -1 // client error
}

func TestNewProxy(t *testing.T) {
	orch := testable.NewMeshOrchestrator()

	testMeshLeft := orch.NewMeshNode("000001", "left")
	testMeshRight := orch.NewMeshNode("000002", "right")

	proxyLeft, err := NewProxy(testMeshLeft, testable.MustConfigManager(testDefaultConfigYAML))
	assert.Nil(t, err)
	assert.NotNil(t, proxyLeft)

	proxyRight, err := NewProxy(testMeshRight, testable.MustConfigManager(testDefaultConfigYAML))
	assert.Nil(t, err)
	assert.NotNil(t, proxyLeft)

	go func() {
		_ = core.RunInterruptibleContext(context.Background(), proxyLeft, proxyRight)
	}()

	client := NewProxyClient("proxy.left", &testable.Doer{
		Handler: func(w http.ResponseWriter, r *http.Request) {
			proxyLeft.ServeHTTP(w, r)
		},
	})

	err = client.Login("admin", ProxyLoginSecret)
	assert.NoError(t, err)

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

	// TODO: implement api requests to the proxy

}
