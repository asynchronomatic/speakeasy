package modeldex

import (
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

type LocalRoute struct {
	BaseURL string
	Token   string
}
type ModelProvider struct {
	ID       string
	Private  bool
	Provider string
	BaseURL  string
	Token    string
}

type ModelRoute struct {
	Name          string // Model Alias
	Model         string // Actual Model name
	Capabilities  []string
	ContextLength int
	Owner         string
	ModifiedAt    time.Time

	// private: these should never go across the wire
	providers []ModelProvider          // local providers of this model
	peers     map[string]core.PeerNode // peernodes that contain this model
}

func (r *ModelRoute) IsLocal() bool {
	return len(r.providers) != 0
}

func (r *ModelRoute) IsPrivate() bool {
	for _, provider := range r.providers {
		if provider.Private {
			return true
		}
	}
	return false
}

func (r *ModelRoute) GetMeshPeerRoute() *core.PeerNode {
	for id := range r.peers {
		node := r.peers[id]
		return &node
	}
	return nil
}

func (r *ModelRoute) GetLocalRoute() *LocalRoute {
	if len(r.providers) == 0 {
		return nil
	}

	return &LocalRoute{
		BaseURL: r.providers[0].BaseURL,
		Token:   r.providers[0].Token,
	}
}

// GetLocalRouteProtected returns a LocalRoute to the first non-private provider if isMesh is true, else any provider is valid.
func (r *ModelRoute) GetLocalRouteProtected(isFromMesh bool) *LocalRoute {
	if len(r.providers) == 0 {
		return nil
	}

	for _, p := range r.providers {
		// if this provider is private and the request is from the mesh
		// do not return it... we need a public one
		if p.Private && isFromMesh {
			continue
		}

		return &LocalRoute{
			BaseURL: p.BaseURL,
			Token:   p.Token,
		}
	}

	return nil
}

func (r *ModelRoute) AddPeer(peer core.PeerNode) {
	if r.peers == nil {
		r.peers = make(map[string]core.PeerNode)
	}
	r.peers[peer.ID] = peer
}

func (r *ModelRoute) RemovePeer(peer core.PeerNode) {
	delete(r.peers, peer.ID)
}

func (r *ModelRoute) IsAvailable() bool {
	return len(r.peers) != 0 || len(r.providers) != 0
}

func (r *ModelRoute) GetPeers() []core.PeerNode {
	peers := make([]core.PeerNode, 0, len(r.peers))
	for id := range r.peers {
		peers = append(peers, r.peers[id])
	}
	return peers
}

func MakeRoute(name, model string, capabilities []string) ModelRoute {
	return ModelRoute{
		Name:         name,
		Model:        model,
		Capabilities: capabilities,
		peers:        make(map[string]core.PeerNode),
		providers:    []ModelProvider{},
	}
}
