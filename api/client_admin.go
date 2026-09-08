package api

import (
	"fmt"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/jsonclient"
)

type AdminClient struct {
	transport jsonclient.Transport
}

// DefaultInviteLifetimeSec is applied when LifetimeSec is 0 and Forever is false.
const DefaultInviteLifetimeSec uint64 = 24 * 60 * 60

type CreateInviteRequest struct {
	Name        string // This will be attached to all nodes invited in (As InvitedAs )
	LifetimeSec uint64 // if LifetimeSec is 0, this invite lives forever
	Reusable    bool   // if this is set invite is single use
	MeshId      string
}

type CreateInviteResponse struct {
	InviteId   string
	InviteLink string
	Reusable   bool
	Expires    int64
}

func (c *AdminClient) CreateInvite(req CreateInviteRequest) (*CreateInviteResponse, error) {
	if req.MeshId == "" {
		req.MeshId = "default"
	}
	resp := CreateInviteResponse{}
	err := c.transport.Post("/api/v1/admin/invite", &req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

/*
func (c *AdminClient) InviteLink(meshId string, lifetime time.Duration) (string, string, error) {
	req := CreateInviteRequest{MeshId: meshId}
	if lifetime > 0 {
		req.LifetimeSec = uint64(lifetime.Seconds())
	}
	resp, err := c.CreateInvite(req)
	if err != nil {
		return "", "", err
	}
	return resp.InviteId, resp.InviteLink, nil
}*/

type DeleteInviteRequest struct {
	Invite string
}

func (c *AdminClient) DeleteInvite(inviteId string) error {
	return c.transport.Delete(fmt.Sprintf("/api/v1/admin/invite/%s", inviteId))
}

type InviteInfo struct {
	InviteId   string
	InviteLink string
	Name       string
	Reusable   bool
	Expires    int64
	MeshId     string
}

type ListInvitesResponse struct {
	Invites []InviteInfo
}

func (c *AdminClient) ListInvites() (*ListInvitesResponse, error) {
	resp := ListInvitesResponse{}
	if err := c.transport.Get("/api/v1/admin/invite", &resp); err != nil {
		return nil, err
	}
	if resp.Invites == nil {
		resp.Invites = []InviteInfo{}
	}
	return &resp, nil
}

type AdminNode struct {
	ID        string
	Name      string
	MeshId    string
	AddedAt   time.Time
	InvitedAs string
}

type ListAdminNodesResponse struct {
	Nodes []AdminNode
}

func (c *AdminClient) ListNodes() (*ListAdminNodesResponse, error) {
	resp := ListAdminNodesResponse{}
	if err := c.transport.Get("/api/v1/admin/nodes", &resp); err != nil {
		return nil, err
	}
	if resp.Nodes == nil {
		resp.Nodes = []AdminNode{}
	}
	return &resp, nil
}

type KickPeerResponse struct {
	NodeID string
}

type DeleteNodeResponse struct {
	NodeID string
}

func (c *AdminClient) DeleteNode(nodeId string) error {
	return c.transport.Delete(fmt.Sprintf("/api/v1/admin/nodes/%s", nodeId))
}

func (c *AdminClient) KickPeer(peerId string) error {
	return c.DeleteNode(peerId)
}

type RedeemInviteRequest struct {
	Node Node
}

type RedeemInviteResponse struct {
	MeshId     string // the Mesh Id to set
	MeshSecret string // the mesh secret to set
	MeshServer string // the mesh server
}
