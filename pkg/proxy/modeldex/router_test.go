package modeldex

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/core"
)

var initialProviders = []config.Provider{
	{
		ID:        "test",
		Type:      "test",
		BaseURL:   "http:://self.mesh",
		Token:     "test-token",
		Private:   true,
		Discovery: "whitelist",
		Models: []config.ModelConfig{
			{
				Model:   "test-model",
				Private: true,
			},
		},
	},
}

var peer = core.NewPeerNode("peer", "Peer")

var initialPeerUpdate = map[string]ModelRoute{
	"test-model": {
		Name:          "test-model",
		Model:         "test-model",
		Capabilities:  []string{},
		ContextLength: 100,
		Owner:         "",
		ModifiedAt:    time.Time{},
		providers:     nil,
		peers: map[string]core.PeerNode{
			"peer": peer,
		},
	},
}

var secondPeerUpdate = map[string]ModelRoute{
	"test-model-2": {
		Name:          "test-model-2",
		Model:         "test-model-2",
		Capabilities:  []string{},
		ContextLength: 100,
		Owner:         "",
		ModifiedAt:    time.Time{},
		providers:     nil,
		peers: map[string]core.PeerNode{
			"peer": peer,
		},
	},
}

var swapPeerUpdate = map[string]ModelRoute{
	"test-model-3": {
		Name:          "test-model-3",
		Model:         "test-model-3",
		Capabilities:  []string{},
		ContextLength: 100,
		Owner:         "",
		ModifiedAt:    time.Time{},
		providers:     nil,
		peers: map[string]core.PeerNode{
			"peer": peer,
		},
	},
}

func TestRouter(t *testing.T) {
	var err error
	node := core.NewPeerNode("self", "Self")

	d := NewModelDiscovery(node, initialProviders, nil)

	d.Refresh()

	models := d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 1, len(models))
	assert.Equal(t, "test-model", models[0].Model)
	assert.Equal(t, 1, len(models[0].providers))
	assert.Equal(t, 1, len(models[0].peers))

	d.UpdatePeerModels(peer, initialPeerUpdate)

	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 1, len(models))
	assert.Equal(t, "test-model", models[0].Model)
	assert.Equal(t, 1, len(models[0].providers))
	assert.Equal(t, 2, len(models[0].peers))

	d.RemoveProvider(initialProviders[0])

	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 1, len(models))
	assert.Equal(t, "test-model", models[0].Model)
	assert.Equal(t, 1, len(models[0].peers))
	assert.Equal(t, peer, models[0].peers[peer.ID])
	assert.Equal(t, 0, len(models[0].providers))

	d.RemovePeer(peer)
	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 0, len(models))

	err = d.AddProvider(initialProviders[0])
	assert.NoError(t, err)
	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 1, len(models))
	assert.Equal(t, "test-model", models[0].Model)
	assert.Equal(t, 1, len(models[0].providers))
	assert.Equal(t, 1, len(models[0].peers))

	d.UpdatePeerModels(peer, secondPeerUpdate)
	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 2, len(models))

	d.UpdatePeerModels(peer, swapPeerUpdate)
	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 2, len(models))
}
