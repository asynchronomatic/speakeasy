package modeldex

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/core"
)

func TestRouter(t *testing.T) {
	node := core.NewPeerNode("self", "Self")

	d := NewModelDiscovery(node, []config.Provider{
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
	}, nil)

	d.Refresh()

	models := d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 1, len(models))

	assert.Equal(t, "test-model", models[0].Model)
	assert.Equal(t, 1, len(models[0].providers))
	assert.Equal(t, 1, len(models[0].peers))

	peer := core.NewPeerNode("peer", "Peer")
	d.AddPeerModels(peer, map[string]ModelRoute{
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
	})

	models = d.ListMeshModels()
	assert.NotNil(t, models)
	assert.Equal(t, 1, len(models))
	assert.Equal(t, "test-model", models[0].Model)
	assert.Equal(t, 1, len(models[0].providers))
	assert.Equal(t, 2, len(models[0].peers))

}
