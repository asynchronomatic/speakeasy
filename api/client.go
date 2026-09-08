package api

import (
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

// Client allows for access to the relay control server for managing mes ACLs
type Client struct {
	transport jsonrpc.Transport
}

// Authorize this client for access to the mesh
func (c *Client) Authorize(id string) error {
	req := RegisterNodeRequest{
		Node: Node{
			ID: id,
		},
	}

	var resp Node
	return c.transport.Post("/api/v1/authorize", &req, &resp)
}

type MeshDetails struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type MeshListResponse struct {
	Meshes []MeshDetails `json:"data"`
}

func (c *Client) Admin() *AdminClient {
	return &AdminClient{
		transport: c.transport,
	}
}

// MeshList returns the meshes you have access to
func (c *Client) MeshList() []MeshDetails {
	return []MeshDetails{
		{
			ID:          "default",
			Description: "Default mesh",
		},
	}
}

func (c *Client) Mesh(meshId string) (*MeshClient, error) {
	return &MeshClient{
		transport: c.transport,
		meshId:    meshId,
	}, nil
}

// NewClient creates a new client for interacting with the mesh relay control server
func NewClient(address, token string) *Client {
	transport := jsonrpc.NewClient(address, token)
	c := &Client{
		transport: transport,
	}
	return c
}
