package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/negrel/assert"
	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
)

type inviteSecret struct {
	UUID     string
	MeshId   string
	Reusable bool
	Expires  int64
	InviteAs string
}

// workflow:
//  create an invite link
//  invite link is sent to a user
//  user passes invite link to ./mesh join
//  ./mesh join does http.get(link)
//  receives the node id, the server config, and a private token for logins
//  writes confi

func newInviteID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func invitePublicID(uuid string) string {
	sum := sha256.Sum256([]byte(uuid))
	return hex.EncodeToString(sum[:])
}

func inviteKVKey(meshID, inviteID string) string {
	return "/invites/" + meshID + "/" + inviteID
}

type meshNodeRecord struct {
	NodeID       string
	Name         string
	AddedAt      time.Time
	InvitedAs    string
	PasswordHash string
}

func newNodeLoginSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashLoginSecret(secret string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func meshNodeKVKey(meshID, nodeID string) string {
	return "/mesh/" + meshID + "/nodes/" + nodeID
}

func (s *Server) adminCreateInviteLink(ctx *JsonRPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	req := api.CreateInviteRequest{}
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.MeshId == "" {
		return api.NewError(http.StatusBadRequest, "mesh id is required")
	}

	// FIXME: we only have the default mesh
	req.MeshId = "default"

	// Invite codes are stored under /invites/<meshID>/<code>.
	// Defaults: one-time, 24h expiry. Forever/reusable require explicit flags.

	inviteUUID, err := newInviteID()
	if err != nil {
		return err
	}

	invite := inviteSecret{
		UUID:     inviteUUID,
		Reusable: req.Reusable,
		MeshId:   req.MeshId,
		InviteAs: req.Name,
	}
	if req.LifetimeSec != 0 {
		invite.Expires = time.Now().Add(time.Duration(req.LifetimeSec) * time.Second).Unix()
	}

	inviteID := invitePublicID(inviteUUID)
	if err := s.kv.Put(inviteKVKey(req.MeshId, inviteID), invite); err != nil {
		return err
	}

	base := strings.TrimRight(s.advertiseURL, "/")
	resp := api.CreateInviteResponse{
		InviteId:   inviteID,
		InviteLink: fmt.Sprintf("%s/api/v1/redeem/%s", base, inviteID),
		Reusable:   invite.Reusable,
		Expires:    invite.Expires,
	}
	return ctx.ReplyObject(&resp)
}

func (s *Server) adminRedeemInviteLink(ctx *JsonRPC) error {
	inviteID := ctx.PathVar("id")
	if inviteID == "" {
		return api.NewError(http.StatusBadRequest, "invite id is required")
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// FIXME: we only have the default mesh
	key := inviteKVKey("default", inviteID)
	var invite inviteSecret
	if err := s.kv.Get(key, &invite); err != nil {
		if errors.Is(err, jsonkv.ErrNotFound) {
			return api.NewError(http.StatusNotFound, "invite not found")
		}
		return err
	}

	if invite.Expires != 0 && time.Now().Unix() >= invite.Expires {
		_ = s.kv.Delete(key)
		return api.NewError(http.StatusGone, "invite expired")
	}

	req := api.RedeemInviteRequest{}
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.Node.ID == "" {
		return api.NewError(http.StatusBadRequest, "node peer id is required")
	}
	if req.Node.Name == "" {
		req.Node.Name = invite.InviteAs
	}

	s.acl.Add(req.Node.ID)

	secret, err := newNodeLoginSecret()
	if err != nil {
		return err
	}
	hash, err := hashLoginSecret(secret)
	if err != nil {
		return err
	}

	if err := s.kv.Put(meshNodeKVKey(invite.MeshId, req.Node.ID), meshNodeRecord{
		NodeID:       req.Node.ID,
		Name:         req.Node.Name,
		AddedAt:      time.Now().UTC(),
		InvitedAs:    invite.InviteAs,
		PasswordHash: hash,
	}); err != nil {
		return err
	}

	if !invite.Reusable {
		if err := s.kv.Delete(key); err != nil {
			return err
		}
	}

	resp := api.RedeemInviteResponse{
		MeshId:     invite.MeshId,
		MeshSecret: secret,
		MeshServer: s.advertiseURL,
	}
	return ctx.ReplyObject(&resp)
}

func (s *Server) adminListInviteLinks(ctx *JsonRPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	prefix := "/invites/default/"
	base := strings.TrimRight(s.advertiseURL, "/")
	now := time.Now().Unix()
	invites := make([]api.InviteInfo, 0)
	var expired []string

	err := s.kv.ForEach(prefix, func(key string, data []byte) error {
		var invite inviteSecret
		if err := json.Unmarshal(data, &invite); err != nil {
			return err
		}
		id := strings.TrimPrefix(key, prefix)
		if id == "" {
			return nil
		}
		if invite.Expires != 0 && now >= invite.Expires {
			expired = append(expired, key)
			return nil
		}
		invites = append(invites, api.InviteInfo{
			InviteId:   id,
			InviteLink: fmt.Sprintf("%s/api/v1/redeem/%s", base, id),
			Name:       invite.InviteAs,
			Reusable:   invite.Reusable,
			Expires:    invite.Expires,
			MeshId:     invite.MeshId,
		})
		return nil
	})
	if err != nil {
		return err
	}
	for _, key := range expired {
		_ = s.kv.Delete(key)
	}
	return ctx.ReplyObject(&api.ListInvitesResponse{Invites: invites})
}

func parseMeshNodeKVKey(key string) (meshID, nodeID string, ok bool) {
	const prefix = "/mesh/"
	if !strings.HasPrefix(key, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
	if len(parts) != 3 || parts[1] != "nodes" || parts[0] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

// adminListNodes lists nodes stored in the database across every mesh.
func (s *Server) adminListNodes(ctx *JsonRPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	nodes := make([]api.AdminNode, 0)
	err := s.kv.ForEach("/mesh/", func(key string, data []byte) error {
		meshID, nodeID, ok := parseMeshNodeKVKey(key)
		if !ok {
			return nil
		}
		var rec meshNodeRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		id := rec.NodeID
		if id == "" {
			id = nodeID
		}
		nodes = append(nodes, api.AdminNode{
			ID:        id,
			Name:      rec.Name,
			MeshId:    meshID,
			AddedAt:   rec.AddedAt,
			InvitedAs: rec.InvitedAs,
		})
		return nil
	})
	if err != nil {
		return err
	}
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

func (s *Server) meshKeysForNode(id string) ([]string, error) {
	var keys []string
	err := s.kv.ForEach("/mesh/", func(key string, data []byte) error {
		_, nodeID, ok := parseMeshNodeKVKey(key)
		if !ok {
			return nil
		}
		if nodeID == id {
			keys = append(keys, key)
			return nil
		}
		var rec meshNodeRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		if rec.NodeID == id {
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}

// adminDeleteNode removes a node from every mesh in the database and from the allow list.
func (s *Server) adminDeleteNode(ctx *JsonRPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	id := ctx.PathVar("id")
	if id == "" {
		return api.NewError(http.StatusBadRequest, "node id is required")
	}

	keys, err := s.meshKeysForNode(id)
	if err != nil {
		return err
	}

	s.lock.Lock()
	_, registered := s.nodes[id]
	if registered {
		s.lastUpdate = time.Now()
		s.logicalTime++
		delete(s.nodes, id)
	}
	s.lock.Unlock()

	s.acl.Remove(id)

	if len(keys) == 0 && !registered {
		return api.NewError(http.StatusNotFound, "node not found")
	}
	for _, key := range keys {
		if err := s.kv.Delete(key); err != nil {
			return err
		}
	}

	return ctx.ReplyObject(&api.DeleteNodeResponse{NodeID: id})
}

func (s *Server) adminKickPeer(ctx *JsonRPC) error {
	return s.adminDeleteNode(ctx)
}

func (s *Server) adminDeleteInviteLink(ctx *JsonRPC) error {
	assert.Equal(AdminGroup, ctx.Group())

	inviteID := ctx.PathVar("id")
	if inviteID == "" {
		return api.NewError(http.StatusBadRequest, "invite id is required")
	}

	// FIXME: we only have the default mesh
	key := inviteKVKey("default", inviteID)
	var invite inviteSecret
	if err := s.kv.Get(key, &invite); err != nil {
		if errors.Is(err, jsonkv.ErrNotFound) {
			return api.NewError(http.StatusNotFound, "invite not found")
		}
		return err
	}

	if err := s.kv.Delete(key); err != nil {
		return err
	}

	return ctx.ReplyObject(&api.DeleteInviteRequest{Invite: inviteID})
}
