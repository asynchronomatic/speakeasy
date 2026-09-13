package testable

import (
	"context"
	"net/http"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

type MeshNode struct {
	node          core.PeerNode
	httpHandler   http.HandlerFunc
	updateHandler core.UpdateHandlerFunc
	orchestrator  *MeshOrchestrator
}

func (t *MeshNode) AdminAddress() string {
	return t.orchestrator.AdminAddress()
}

func (t *MeshNode) Node() core.PeerNode {
	return t.node
}

func (t *MeshNode) Connect(ctx context.Context) error {
	return t.orchestrator.Connect(t)
}

func (t *MeshNode) Disconnect() error {
	return t.orchestrator.Disconnect(t)
}

func (t *MeshNode) SignalUpdate() {
	t.orchestrator.SignalUpdate(t)
}

func (t *MeshNode) ClientForPeer(dest core.PeerNode, longLived bool) jsonrpc.Doer {
	return t.orchestrator.ClientForPeer(dest, longLived)
}

func (t *MeshNode) ProxyToNode(dest core.PeerNode, w http.ResponseWriter, r *http.Request) {
	t.orchestrator.ProxyToNode(dest, w, r)
}

func (t *MeshNode) GetPeerMeshInfo(node core.PeerNode) *core.MeshInfo {
	return nil
}

func (t *MeshNode) GetPeerMap() (map[string]core.PeerNode, error) {
	return t.orchestrator.GetPeerMap()
}

func (t *MeshNode) WithHandlerFunc(h http.HandlerFunc) {
	t.httpHandler = h
}

func (t *MeshNode) WithUpdateHandlerFunc(h core.UpdateHandlerFunc) {
	t.updateHandler = h
}

func (t *MeshNode) onNodeConnected(node core.PeerNode) {
	_ = t.updateHandler(node, false)
}

func (t *MeshNode) onNodeDisconnected(node core.PeerNode) {
	_ = t.updateHandler(node, true)
}

func NewTestableMesh(node core.PeerNode) *MeshNode {
	return &MeshNode{
		node:          node,
		updateHandler: nil,
		httpHandler:   nil,
	}
}
