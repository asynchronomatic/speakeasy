package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonclient"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

type ExportedModel struct {
	Name       string // Model Alias
	Model      string // Actual Model name
	Properties api.ListModelResponse
	Process    api.ProcessModelResponse
}

type Node = core.PeerNode

type RegisterNodeRequest struct {
	Node        Node
	Token       string
	LogicalTime uint64
	LastUpdate  time.Time
}

type ListNodesResponse struct {
	Nodes []Node
}

type GetRelayResponse struct {
	ID           string
	MultiAddress []string // the relay multiaddress
	LastUpdate   time.Time
	LogicalTime  uint64
}

type NodeLoginRequest struct {
	NodeID     string
	MeshId     string
	MeshSecret string
}

type NodeLoginResponse struct {
	Token   string
	NodeID  string
	Expires int64
}

type MeshClient struct {
	meshId    string
	transport jsonclient.Transport
}

type Registration struct {
	transport jsonclient.Transport
	node      Node
	token     string
}

type RegisterNodeResponse = RegisterNodeRequest

func (r *Registration) Refresh() (bool, uint64, error) {
	req := RegisterNodeRequest{
		Node:  r.node,
		Token: r.token,
	}

	resp := RegisterNodeResponse{}

	// refreshes the node
	err := r.transport.Post(fmt.Sprintf("/api/v1/nodes/%s", r.node.ID), &req, &resp)
	if err != nil {
		log.Errorf("failed to refresh node:  %v %v", req, err)
		if strings.Contains(err.Error(), "409") {
			return false, 0, nil
		}
		return false, 0, err
	}
	r.token = resp.Token
	return true, resp.LogicalTime, nil
}

// GetPeers returns a list of currently configured peers for our mesh
func (c *MeshClient) GetPeers() ([]Node, error) {
	resp := ListNodesResponse{}

	err := c.transport.Get("/api/v1/nodes", &resp)
	if err != nil {
		return nil, err
	}

	return resp.Nodes, nil
}

// GetRelay returns the p2p relay address for our mesh along with the last modified timestamp
func (c *MeshClient) GetRelay() ([]string, time.Time, uint64, error) {
	resp := GetRelayResponse{}

	err := c.transport.Get("/api/v1/relay", &resp)
	if err != nil {
		return nil, time.Time{}, 0, err
	}

	return resp.MultiAddress, resp.LastUpdate, resp.LogicalTime, nil
}

// GetAddress returns just the Relay p2p address
func (c *MeshClient) GetAddress() ([]string, error) {
	relay, _, _, err := c.GetRelay()
	return relay, err
}

// Login this client for access to the mesh
func (c *MeshClient) Login(nodeID, meshSecret string) error {
	req := NodeLoginRequest{
		NodeID:     nodeID,
		MeshId:     c.meshId,
		MeshSecret: meshSecret,
	}
	resp := NodeLoginResponse{}
	if err := c.transport.Post("/api/v1/login", &req, &resp); err != nil {
		return err
	}

	//
	c.transport.SetToken(resp.Token)
	return nil
}

/*
func (c *MeshClient) Authorize(id string) error {
	req := RegisterNodeRequest{
		Node: Node{
			ID: id,
		},
	}
	var resp Node
	return c.transport.Post("/api/v1/authorize", &req, &resp)
}*/

func (c *MeshClient) Unregister(id string) error {
	return c.transport.Delete(fmt.Sprintf("/api/v1/nodes/%s", id))
}

func (c *MeshClient) Register(name string, id string) (*Registration, error) {
	req := RegisterNodeRequest{
		Node: Node{
			Name: name,
			ID:   id,
		},
	}

	resp := RegisterNodeResponse{}
	err := c.transport.Post("/api/v1/nodes", &req, &resp)
	if err != nil {
		return nil, err
	}
	return &Registration{
		transport: c.transport,
		node:      resp.Node,
		token:     resp.Token,
	}, nil
}
