package api

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

// Re-login this far before the session expiry so a 10-minute token
// is refreshed on the next controller poll instead of failing first.
const sessionRefreshSkew = time.Minute

type ExportedModel struct {
	Name       string // Model Alias
	Model      string // Actual Model name
	Properties api.ListModelResponse
	Process    api.ProcessModelResponse
}

type Node = core.PeerNode

type RegisterNodeRequest struct {
	Node        Node
	InstanceID  string
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
	transport jsonrpc.Transport

	mu      sync.Mutex
	nodeID  string
	secret  string
	expires int64
}

type Registration struct {
	client     *MeshClient
	node       Node
	instanceID string // unique id of this registration instance
}

type RegisterNodeResponse = RegisterNodeRequest

func (r *Registration) refresh(updated bool) (bool, uint64, error) {
	if err := r.client.ensureSession(); err != nil {
		return false, 0, err
	}
	req := RegisterNodeRequest{
		Node:       r.node,
		InstanceID: r.instanceID,
	}

	if updated {
		req.LastUpdate = time.Now()
	}

	resp := RegisterNodeResponse{}

	// refreshes the node
	err := r.client.transport.Post(fmt.Sprintf("/api/v1/nodes/%s", r.node.ID), &req, &resp)
	if err != nil {
		log.Errorf("failed to refresh node:  %v %v", req, err)
		if strings.Contains(err.Error(), "409") {
			return false, 0, nil
		}
		return false, 0, err
	}
	r.instanceID = resp.InstanceID
	return true, resp.LogicalTime, nil
}

func (r *Registration) Refresh() (bool, uint64, error) {
	return r.refresh(false)
}

func (r *Registration) SignalUpdate() error {
	_, _, err := r.refresh(true)
	return err
}

// GetPeers returns a list of currently configured peers for our mesh
func (c *MeshClient) GetPeers() ([]Node, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	resp := ListNodesResponse{}

	err := c.transport.Get("/api/v1/nodes", &resp)
	if err != nil {
		return nil, err
	}

	return resp.Nodes, nil
}

// GetRelay returns the p2p relay address for our mesh along with the last modified timestamp
func (c *MeshClient) GetRelay() ([]string, time.Time, uint64, error) {
	if err := c.ensureSession(); err != nil {
		return nil, time.Time{}, 0, err
	}
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
	c.mu.Lock()
	c.nodeID = nodeID
	c.secret = meshSecret
	c.mu.Unlock()
	return c.login()
}

func (c *MeshClient) login() error {
	c.mu.Lock()
	req := NodeLoginRequest{
		NodeID:     c.nodeID,
		MeshId:     c.meshId,
		MeshSecret: c.secret,
	}
	c.mu.Unlock()

	resp := NodeLoginResponse{}
	if err := c.transport.Post("/api/v1/login", &req, &resp); err != nil {
		return err
	}

	c.mu.Lock()
	c.transport.SetToken(resp.Token)
	c.expires = resp.Expires
	c.mu.Unlock()
	return nil
}

func (c *MeshClient) ensureSession() error {
	c.mu.Lock()
	nodeID, secret, expires := c.nodeID, c.secret, c.expires
	c.mu.Unlock()
	if nodeID == "" || secret == "" {
		return nil
	}
	if expires != 0 && time.Now().Add(sessionRefreshSkew).Unix() < expires {
		return nil
	}
	return c.login()
}

func (c *MeshClient) Unregister(id string) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	return c.transport.Delete(fmt.Sprintf("/api/v1/nodes/%s", id))
}

func (c *MeshClient) Register(name string, id string) (*Registration, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
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
		client:     c,
		node:       resp.Node,
		instanceID: resp.InstanceID,
	}, nil
}
