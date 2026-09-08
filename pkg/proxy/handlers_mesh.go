package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

func (p *Proxy) meshStatus(w http.ResponseWriter, r *http.Request) {
	resp := NodeStatusResponse{
		Status: NodeStatus{
			Name:      p.mesh.Node().Name,
			PeerID:    p.mesh.Node().ID,
			Reachable: true,
			Models:    make([]string, 0),
		},
	}

	resp.Status.Models = p.modelRouter.ListModels()
	resp.Status.Mesh = p.mesh.GetPeerMeshInfo(p.mesh.Node()) // inspect self

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&resp)
}

// meshModels is called by a peer node to get this nodes exported(local) models
func (p *Proxy) meshModels(w http.ResponseWriter, r *http.Request) {
	resp := MeshListModelsResponse{
		Models: p.modelRouter.ListExportedModels(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&resp)
}

type MeshMembersResponse struct {
	Nodes []NodeStatus
}

func (p *Proxy) meshMembers(rpc *jsonrpc.RPC) error {
	resp := MeshMembersResponse{}

	peers, err := p.mesh.GetPeerMap()
	if err != nil {
		return jsonrpc.NewError(http.StatusInternalServerError, err.Error())
	}

	for _, peer := range peers {
		var status NodeStatus
		if peer.ID == p.mesh.Node().ID {
			status = NodeStatus{
				Name:      peer.Name,
				PeerID:    p.mesh.Node().ID,
				Reachable: true,
				Type:      "self",
				Models:    p.modelRouter.ListModels(),
				Mesh:      p.mesh.GetPeerMeshInfo(peer),
			}
		} else {
			// FIXME: we can use a long lived connection, but then we need to know if it is long lived or not
			client := NewMeshClient(peer.Name, p.mesh.ClientForPeer(peer, true))
			status, err = client.GetMeshStatus()
			if err != nil {
				log.WithName("proxy").Infof("failed to get mesh status from peer %s: %v", peer, err)
				status.PeerID = peer.ID
				status.Name = peer.Name
				status.Reachable = false
			}
			status.Type = "peer"
		}

		if status.Mesh != nil {
			for idx, c := range status.Mesh.Connections {
				if node, ok := peers[c.PeerID]; ok {
					status.Mesh.Connections[idx].PeerName = node.Name
				}
			}
		}
		resp.Nodes = append(resp.Nodes, status)
	}

	return rpc.ReplyObject(&resp)
}
