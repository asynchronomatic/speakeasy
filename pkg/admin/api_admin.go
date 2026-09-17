package admin

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/negrel/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

const (
	AdminGroup = "admin"
	MeshGroup  = "mesh"
)

func newNodeLoginSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) adminCreateInviteLink(ctx *jsonrpc.RPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	req := api.CreateInviteRequest{}
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.MeshId == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "mesh id is required")
	}

	inviteID, expires, err := s.inviteStore.CreateInvite(req.Name, req.Reusable, req.LifetimeSec)
	if err != nil {
		return err
	}

	base := strings.TrimRight(s.advertiseURL, "/")
	resp := api.CreateInviteResponse{
		InviteId:   inviteID,
		InviteLink: fmt.Sprintf("%s/api/v1/redeem/%s", base, inviteID),
		Reusable:   req.Reusable,
		Expires:    expires,
	}
	return ctx.ReplyObject(&resp)
}

func (s *Server) adminRedeemInviteLink(ctx *jsonrpc.RPC) error {
	inviteID := ctx.PathVar("id")
	if inviteID == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "invite id is required")
	}

	req := api.RedeemInviteRequest{}
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.Node.ID == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node peer id is required")
	}

	resp := api.RedeemInviteResponse{
		MeshId:     "default",
		MeshServer: s.advertiseURL,
	}

	err := s.inviteStore.Redeem(inviteID, func(invite *inviteSecret) error {
		var err error
		if req.Node.Name == "" {
			req.Node.Name = invite.InviteAs
		}

		resp.MeshSecret, err = newNodeLoginSecret()
		if err != nil {
			return err
		}

		err = s.nodeStore.AddNode(req.Node, invite.InviteAs, resp.MeshSecret)
		if err != nil {
			if errors.Is(err, ErrDuplicateNode) {
				return jsonrpc.NewError(http.StatusConflict, err.Error())
			}
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return ctx.ReplyObject(&resp)
}

func (s *Server) adminListInviteLinks(ctx *jsonrpc.RPC) error {
	assert.Equal(AdminGroup, ctx.Group())
	var err error
	resp := api.ListInvitesResponse{}

	if resp.Invites, err = s.inviteStore.ListInvites(strings.TrimRight(s.advertiseURL, "/")); err != nil {
		return err
	}
	return ctx.ReplyObject(&resp)
}

// adminListNodes lists nodes stored in the database across every mesh.
func (s *Server) adminListNodes(ctx *jsonrpc.RPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	nodes := make([]api.AdminNode, 0)
	s.nodeStore.List(func(ref *NodeReference) {
		nodes = append(nodes, api.AdminNode{
			ID:        ref.rec.NodeID,
			Name:      ref.rec.Name,
			MeshId:    "default",
			AddedAt:   ref.rec.AddedAt,
			InvitedAs: ref.rec.InvitedAs,
		})
	})
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].MeshId != nodes[j].MeshId {
			return nodes[i].MeshId < nodes[j].MeshId
		}
		if nodes[i].Name != nodes[j].Name {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].ID < nodes[j].ID
	})
	return ctx.ReplyObject(&api.ListAdminNodesResponse{Nodes: nodes})
}

// adminDeleteNode removes a node from every mesh in the database and from the allow list.
func (s *Server) adminDeleteNode(ctx *jsonrpc.RPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	id := ctx.PathVar("id")
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node id is required")
	}

	// force remove acl always
	err := s.nodeStore.DeleteNode(id)
	if err != nil {
		return jsonrpc.NewError(http.StatusNotFound, err.Error())
	}
	return ctx.ReplyObject(&api.DeleteNodeResponse{NodeID: id})
}

func (s *Server) adminKickPeer(ctx *jsonrpc.RPC) error {
	return s.adminDeleteNode(ctx)
}

func (s *Server) adminDeleteInviteLink(ctx *jsonrpc.RPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	inviteID := ctx.PathVar("id")
	if inviteID == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "invite id is required")
	}

	if err := s.inviteStore.Delete(inviteID); err != nil {
		return err
	}

	return ctx.ReplyObject(&api.DeleteInviteRequest{Invite: inviteID})
}

func (s *Server) AddHybridNode(node core.PeerNode) (string, string, error) {
	secret, err := newNodeLoginSecret()
	if err != nil {
		return "", "", err
	}

	if err = s.nodeStore.AddNode(node, "Bootstrap", secret); err != nil {
		return "", "", err
	}

	return "default", secret, nil
}
