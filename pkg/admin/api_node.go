package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"uuid"

	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
	"github.com/asynchronomatic/speakeasy/pkg/admin/magiclink"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

var SessionTokenTTL = 10 * time.Minute

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

	s.acl.Add(req.Node.ID)
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

	if !s.acl.Has(req.Node.ID) {
		return jsonrpc.NewError(http.StatusBadRequest, "node not Authorized")
	}

	if req.Node.ID != ctx.User() && ctx.Group() != AdminGroup {
		return jsonrpc.NewError(http.StatusBadRequest, "node peer id is bad")
	}

	s.lock.Lock()
	s.lastUpdate = time.Now()
	s.logicalTime++

	req.Node.LastUpdate = s.lastUpdate
	req.Node.LogicalTime = s.logicalTime

	resp := api.RegisterNodeRequest{
		Node: req.Node,
		// this is just needed so that if the node registered we can tell it has a new instance
		// it has nothing to do with auth i'm probably overthinking this
		InstanceID:  uuid.New().String(),
		LastUpdate:  s.lastUpdate,
		LogicalTime: s.logicalTime,
	}

	s.nodes[req.Node.ID] = &NodeReference{
		Node:       req.Node,
		LastPing:   time.Now(),
		InstanceID: resp.InstanceID,
	}
	s.lock.Unlock()
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

	updateNode := func(req *api.RegisterNodeRequest) bool {
		if req.InstanceID == "" {
			log.Errorf("node token is required for %s", id)
			return false
		}

		ref, ok := s.nodes[id]
		if !ok {
			log.Errorf("node not found %s", id)
			return false
		}

		if ref.InstanceID != req.InstanceID {
			log.Errorf("token mismatch in refresh for %s", id)
			return false
		}
		ref.LastPing = time.Now()
		return true
	}

	resp := api.RegisterNodeRequest{
		Node:       req.Node,
		InstanceID: req.InstanceID,
	}
	s.lock.Lock()
	resp.LogicalTime = s.logicalTime
	resp.LastUpdate = s.lastUpdate
	valid := updateNode(&req)
	s.lock.Unlock()

	if !valid {
		return jsonrpc.NewError(http.StatusConflict, "node registration invalid")
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

	s.lock.Lock()
	node, ok := s.nodes[id]
	if ok {
		s.lastUpdate = time.Now()
		s.logicalTime++
	}
	delete(s.nodes, id)
	s.lock.Unlock()

	if !ok {
		return jsonrpc.NewError(http.StatusNotFound, "node not found")
	}

	s.acl.Remove(id)
	return ctx.ReplyObject(&node)
}

func (s *Server) apiNodeList(ctx *jsonrpc.RPC) error {
	s.lock.Lock()
	resp := api.ListNodesResponse{
		Nodes: make([]api.Node, 0, len(s.nodes)),
	}

	for _, ref := range s.nodes {
		resp.Nodes = append(resp.Nodes, ref.Node)
	}
	s.lock.Unlock()

	return ctx.ReplyObject(&resp)
}

func (s *Server) apiRelayGet(ctx *jsonrpc.RPC) error {
	s.lock.Lock()
	resp := api.GetRelayResponse{
		MultiAddress: s.relayAddress,
		LastUpdate:   s.lastUpdate,
		LogicalTime:  s.logicalTime,
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
		return nil, errors.New("session expired")
	}
	if s.acl == nil || !s.acl.Has(claims.NodeID) {
		return nil, errors.New("session revoked")
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

func (s *Server) refreshSessionToken(token string) (string, int64, error) {
	claims, err := s.sessionClaims(token)
	if err != nil {
		return "", 0, err
	}
	return s.issueSessionToken(claims.NodeID, SessionTokenTTL)
}

func (s *Server) apiNodeLogin(ctx *jsonrpc.RPC) error {
	var req api.NodeLoginRequest
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.NodeID == "" || req.MeshSecret == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node id and mesh secret are required")
	}
	ip := security.ClientHost(ctx.Request())
	if security.AuthBlocked(ip) {
		return jsonrpc.NewError(http.StatusTooManyRequests, "too many requests")
	}
	meshID := req.MeshId
	if meshID == "" {
		meshID = "default"
	}

	var rec meshNodeRecord

	if err := s.kv.Get(meshNodeKVKey(meshID, req.NodeID), &rec); err != nil {
		security.DummySecretMatch(req.MeshSecret)
		security.AuthFailure(ip)
		if errors.Is(err, jsonkv.ErrNotFound) {
			return jsonrpc.NewError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}

	mismatch := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(req.MeshSecret)) != nil
	if mismatch {
		security.AuthFailure(ip)
		return jsonrpc.NewError(http.StatusUnauthorized, "invalid credentials")
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

// nodeExpiryCheckInterval defines the interval for checking and expiring stale node registrations.
const nodeExpiryCheckInterval = 5 * time.Minute

// nodeExpiry defines the duration after which a node is considered stale and eligible for expiration.
const nodeExpiry = 15 * time.Minute

func (s *Server) runExpireNodes(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(nodeExpiryCheckInterval):
			now := time.Now()
			s.lock.Lock()
			for k, v := range s.nodes {
				if now.Sub(v.LastPing) > nodeExpiry {
					log.WithName("admin").Infof("expiring stale registration for node %s", k)
					delete(s.nodes, k)
				}
			}
			s.lock.Unlock()
		}
	}
}
