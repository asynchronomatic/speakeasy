package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"uuid"

	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
	"github.com/asynchronomatic/speakeasy/pkg/admin/magiclink"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

var SessionTokenTTL = 10 * time.Minute

// dummyLoginHash is a real bcrypt hash so unknown-node logins pay the same
// CompareHashAndPassword cost as a wrong password. The dummy secret is never
// accepted: login still fails when the node record is missing.
var dummyLoginHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("speakeasy-dummy-login-not-a-real-secret"), bcrypt.DefaultCost)
	if err != nil {
		panic("admin: dummy login hash: " + err.Error())
	}
	dummyLoginHash = h
}

// @deprecated nobody should be calling this anymore
func (s *Server) apiNodeAuthorize(ctx *jsonrpc.RPC) error {
	var req api.RegisterNodeRequest
	if err := ctx.GetObject(&req); err != nil {
		return jsonrpc.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Node.ID == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node peer id is required")
	}
	// only session node or admin can authorize
	if req.Node.ID != ctx.User() && ctx.Group() != AdminGroup {
		return jsonrpc.NewError(http.StatusBadRequest, "node peer id is bad")
	}

	s.nodeStore.AddNode(req.Node, "", "")
	return ctx.ReplyObject(&req.Node)
}

// TODO: nodes need to expire if we have not heard from them in a while
func (s *Server) apiNodeRegister(ctx *jsonrpc.RPC) error {
	var req api.RegisterNodeRequest
	if err := ctx.GetObject(&req); err != nil {
		return jsonrpc.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Node.Name == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node name is required")
	}
	if req.Node.ID == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node peer id is required")
	}

	if req.Node.ID != ctx.User() && ctx.Group() != AdminGroup {
		return jsonrpc.NewError(http.StatusBadRequest, "node peer id is bad")
	}

	instanceId, logicalTime, err := s.nodeStore.Register(req.Node.ID)
	if err != nil {
		return jsonrpc.NewError(http.StatusBadRequest, err.Error())
	}

	req.Node.InstanceId = instanceId
	req.Node.LastUpdate = time.Now() // deprecate
	req.Node.LogicalTime = logicalTime

	resp := api.RegisterNodeRequest{
		Node: req.Node,
		// this is just needed so that if the node registered we can tell it has a new instance
		// it has nothing to do with auth i'm probably overthinking this
		InstanceID:  uuid.New().String(),
		LastUpdate:  req.Node.LastUpdate, // deprecate
		LogicalTime: req.Node.LogicalTime,
	}
	log.Infof("registered node %s", resp.Node.ID)
	return ctx.ReplyObject(&resp)
}

func (s *Server) apiNodeRefresh(ctx *jsonrpc.RPC) error {
	id := ctx.PathVar("id")
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node id is required")
	}

	// only the node logged in can perform this action
	if id != ctx.User() {
		return jsonrpc.NewError(http.StatusBadRequest, "invalid node")
	}

	req := api.RegisterNodeRequest{}
	if err := ctx.GetObject(&req); err != nil {
		return jsonrpc.NewError(http.StatusBadRequest, err.Error())
	}

	if req.Node.ID != id {
		return jsonrpc.NewError(http.StatusBadRequest, "node id mismatch")
	}

	invalidate := false
	if !req.LastUpdate.IsZero() {
		invalidate = true
	}

	valid, _ := s.nodeStore.RefreshNode(id, req.InstanceID, invalidate)
	if valid {
		return jsonrpc.NewError(http.StatusConflict, "node registration invalid")
	}

	resp := api.RegisterNodeRequest{
		Node:        req.Node,
		InstanceID:  req.InstanceID,
		LogicalTime: s.nodeStore.LogicalTime(),
	}

	return ctx.ReplyObject(&resp)
}

// apiNodeUnregister does not actually leave the mesh, it just kicks itself of the network and
// may rejoin later with its given login token
func (s *Server) apiNodeUnregister(ctx *jsonrpc.RPC) error {
	id := ctx.PathVar("id")
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node id is required")
	}

	// only the node logged in can Unregister itself
	if id != ctx.User() && ctx.Group() != AdminGroup {
		return jsonrpc.NewError(http.StatusBadRequest, "invalid node")
	}

	if err := s.nodeStore.Unregister(id); err != nil {
		return jsonrpc.NewError(http.StatusNotFound, "node not found")
	}
	node := api.Node{
		ID: id,
	}
	return ctx.ReplyObject(&node)
}

func (s *Server) apiNodeList(ctx *jsonrpc.RPC) error {
	resp := api.ListNodesResponse{
		Nodes: s.nodeStore.NodeList(),
	}

	s.nodeStore.List(func(ref *NodeReference) {
		resp.Nodes = append(resp.Nodes, ref.Node)
	})

	return ctx.ReplyObject(&resp)
}

func (s *Server) apiRelayGet(ctx *jsonrpc.RPC) error {
	s.lock.Lock()
	resp := api.GetRelayResponse{
		MultiAddress: s.relayAddress,
		LogicalTime:  s.nodeStore.LogicalTime(),
	}
	s.lock.Unlock()

	return ctx.ReplyObject(&resp)
}

type sessionClaims struct {
	NodeID  string
	Expires int64
}

func (s *Server) issueSessionToken(nodeID string, lifetime time.Duration) (string, int64, error) {
	if lifetime <= 0 {
		lifetime = SessionTokenTTL
	}
	claims := sessionClaims{
		NodeID:  nodeID,
		Expires: time.Now().Add(lifetime).Unix(),
	}
	raw, err := magiclink.New(s.magicKey).Encrypt(&claims)
	if err != nil {
		return "", 0, err
	}
	return auth.SessionTokenPrefix + raw, claims.Expires, nil
}

func (s *Server) sessionClaims(token string) (*sessionClaims, error) {
	if !strings.HasPrefix(token, auth.SessionTokenPrefix) {
		return nil, errors.New("not a session token")
	}
	var claims sessionClaims
	payload := strings.TrimPrefix(token, auth.SessionTokenPrefix)
	if err := magiclink.New(s.magicKey).Decrypt(payload, &claims); err != nil {
		return nil, err
	}
	if claims.NodeID == "" {
		return nil, errors.New("invalid session")
	}
	if claims.Expires == 0 || time.Now().Unix() >= claims.Expires {
		log.WithName("auth").Warnf("session expired %s", claims.NodeID)
		return nil, errors.New("session expired")
	}
	if !s.GetAllowList().Has(claims.NodeID) {
		log.WithName("auth").Warnf("node not authorized %s", claims.NodeID)
		return nil, jsonrpc.NewError(http.StatusForbidden, "no authorization")
	}
	return &claims, nil
}

func (s *Server) authenticateSessionToken(token string) (*auth.Properties, error) {
	claims, err := s.sessionClaims(token)
	if err != nil {
		return nil, err
	}
	return &auth.Properties{User: claims.NodeID, Group: MeshGroup}, nil
}

func (s *Server) apiNodeLogin(ctx *jsonrpc.RPC) error {
	var req api.NodeLoginRequest
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.NodeID == "" || req.MeshSecret == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node id and mesh secret are required")
	}
	meshID := req.MeshId
	if meshID == "" {
		meshID = "default"
	}

	if err := s.nodeStore.NodeLogin(req.NodeID, req.MeshSecret); err != nil {
		return err
	}

	token, expires, err := s.issueSessionToken(req.NodeID, SessionTokenTTL)
	if err != nil {
		return err
	}

	return ctx.ReplyObject(&api.NodeLoginResponse{
		Token:   token,
		NodeID:  req.NodeID,
		Expires: expires,
	})
}
