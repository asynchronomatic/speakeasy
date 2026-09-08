package proxy

import (
	"fmt"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/proxy/modeldex"
)

// MeshClient for making api calls across the mesh network
type MeshClient struct {
	address string
	client  jsonrpc.Doer
}

type MeshListModelsResponse struct {
	Models []modeldex.ModelRoute `json:"models"`
}

// GetModelsMesh gets models over the mesh ( internal RPC )
func (c *MeshClient) GetModelsMesh() (map[string]modeldex.ModelRoute, error) {
	resp := MeshListModelsResponse{}

	jc := jsonrpc.NewClient(fmt.Sprintf("http://%s.mesh", c.address), "").WithDoer(c.client)
	err := jc.Get("/.mesh/models", &resp)
	if err != nil {
		return nil, err
	}

	models := make(map[string]modeldex.ModelRoute)

	for _, m := range resp.Models {
		models[m.Name] = m
	}

	return models, nil
}

type NodeStatus struct {
	Name      string
	PeerID    string
	Type      string
	Reachable bool
	Models    []string
	Mesh      *core.MeshInfo
}

type NodeStatusResponse struct {
	Status NodeStatus
}

func (c *MeshClient) GetMeshStatus() (NodeStatus, error) {
	resp := NodeStatusResponse{}

	jc := jsonrpc.NewClient(fmt.Sprintf("http://%s.mesh", c.address), "").WithDoer(c.client)
	err := jc.Get("/.mesh/status", &resp)
	return resp.Status, err
}

// NewMeshClient creates a client to the ollama interface wrapping the api into our model format
// this client can also use a custom http.Client which is connected over our peer network
func NewMeshClient(address string, client jsonrpc.Doer) *MeshClient {
	c := &MeshClient{
		address: address,
		client:  client,
	}
	return c
}
