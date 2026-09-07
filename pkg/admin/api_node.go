package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
	"github.com/asynchronomatic/speakeasy/pkg/admin/magiclink"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

var SessionTokenTTL = time.Duration(10 * time.Minute)

func newNodeToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

func (s *Server) apiNodeAuthorize(ctx *JsonRPC) error {
	var req api.RegisterNodeRequest
	if err := ctx.GetObject(&req); err != nil {
		return api.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Node.ID == "" {
		return api.NewError(http.StatusBadRequest, "node peer id is required")
	}

	s.acl.Add(req.Node.ID)
	return ctx.ReplyObject(&req.Node)
}

// TODO: nodes need to expire if we have not heard from them in a while
func (s *Server) apiNodeRegister(ctx *JsonRPC) error {
	var req api.RegisterNodeRequest
	if err := ctx.GetObject(&req); err != nil {
		return api.NewError(http.StatusBadRequest, err.Error())
	}
	if req.Node.Name == "" {
		return api.NewError(http.StatusBadRequest, "node name is required")
	}
	if req.Node.ID == "" {
		return api.NewError(http.StatusBadRequest, "node peer id is required")
	}

	if !s.acl.Has(req.Node.ID) {
		return api.NewError(http.StatusBadRequest, "node not Authorized")
	}

	s.lock.Lock()
	s.lastUpdate = time.Now()
	s.logicalTime++

	req.Node.LastUpdate = s.lastUpdate
	req.Node.LogicalTime = s.logicalTime

	resp := api.RegisterNodeRequest{
		Node:        req.Node,
		Token:       newNodeToken(),
		LastUpdate:  s.lastUpdate,
		LogicalTime: s.logicalTime,
	}

	s.nodes[req.Node.ID] = &NodeReference{
		Node:     req.Node,
		LastPing: time.Now(),
		Token:    resp.Token,
	}
	s.lock.Unlock()
	log.Infof("registered node %s", resp.Node.ID)

	return ctx.ReplyObject(&resp)
}

func (s *Server) apiNodeRefresh(ctx *JsonRPC) error {
	id := ctx.PathVar("id")
	if id == "" {
		return api.NewError(http.StatusBadRequest, "node id is required")
	}

	req := api.RegisterNodeRequest{}
	if err := ctx.GetObject(&req); err != nil {
		return api.NewError(http.StatusBadRequest, err.Error())
	}

	if req.Node.ID != id {
		return api.NewError(http.StatusBadRequest, "node id mismatch")
	}

	updateNode := func(req *api.RegisterNodeRequest) bool {
		if req.Token == "" {
			log.Errorf("node token is required")
			return false
		}

		ref, ok := s.nodes[id]
		if !ok {
			log.Errorf("node not found %s", id)
			return false
		}

		if ref.Token != req.Token {
			log.Errorf("token mismatch %s %s", ref.Token, req.Token)
			return false
		}
		ref.LastPing = time.Now()
		return true
	}

	resp := api.RegisterNodeRequest{
		Node:  req.Node,
		Token: req.Token,
	}
	s.lock.Lock()
	resp.LogicalTime = s.logicalTime
	resp.LastUpdate = s.lastUpdate
	valid := updateNode(&req)
	s.lock.Unlock()

	if !valid {
		return api.NewError(http.StatusConflict, "node registration invalid")
	}

	return ctx.ReplyObject(&resp)
}

func (s *Server) apiNodeUnregister(ctx *JsonRPC) error {
	id := ctx.PathVar("id")
	if id == "" {
		return api.NewError(http.StatusBadRequest, "node id is required")
	}

	s.lock.Lock()
	node, ok := s.nodes[id]
	if ok {
		s.lastUpdate = time.Now()
		s.logicalTime++
	}
	delete(s.nodes, id)
	s.lock.Unlock()

	s.acl.Remove(id) // can be done outside the lock
	log.Infof("unregistered node %s", id)

	if !ok {
		return api.NewError(http.StatusNotFound, "node not found")
	}
	return ctx.ReplyObject(&node)
}

func (s *Server) apiNodeList(ctx *JsonRPC) error {
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

func (s *Server) apiRelayGet(ctx *JsonRPC) error {
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
	claims := sessionClaims{NodeID: nodeID}
	if lifetime > 0 {
		claims.Expires = time.Now().Add(lifetime).Unix()
	}
	raw, err := magiclink.New(s.magicKey).Encrypt(&claims)
	if err != nil {
		return "", 0, err
	}
	return auth.SessionTokenPrefix + raw, claims.Expires, nil
}

func (s *Server) authenticateSessionToken(token string) (*auth.Properties, error) {
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
	if claims.Expires != 0 && time.Now().Unix() >= claims.Expires {
		return nil, errors.New("session expired")
	}
	return &auth.Properties{User: claims.NodeID, Group: "mesh"}, nil
}

func (s *Server) refreshSessionToken(token string) (string, int64, error) {
	if !strings.HasPrefix(token, auth.SessionTokenPrefix) {
		return "", 0, errors.New("not a session token")
	}
	var claims sessionClaims
	payload := strings.TrimPrefix(token, auth.SessionTokenPrefix)
	if err := magiclink.New(s.magicKey).Decrypt(payload, &claims); err != nil {
		return "", 0, err
	}
	if claims.NodeID == "" {
		return "", 0, errors.New("invalid session")
	}
	if claims.Expires != 0 && time.Now().Unix() >= claims.Expires {
		return "", 0, errors.New("session expired")
	}
	return s.issueSessionToken(claims.NodeID, SessionTokenTTL)
}

func (s *Server) apiNodeLogin(ctx *JsonRPC) error {
	var req api.NodeLoginRequest
	if err := ctx.GetObject(&req); err != nil {
		return err
	}
	if req.NodeID == "" || req.MeshSecret == "" {
		return api.NewError(http.StatusBadRequest, "node id and mesh secret are required")
	}
	meshID := req.MeshId
	if meshID == "" {
		meshID = "default"
	}

	var rec meshNodeRecord
	if err := s.kv.Get(meshNodeKVKey(meshID, req.NodeID), &rec); err != nil {
		if errors.Is(err, jsonkv.ErrNotFound) {
			return api.NewError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}
	if rec.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(req.MeshSecret)) != nil {
		return api.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	// FIXME:  the session never expires but that's just bad, we should expire the session by requiring the
	//         the client to once in a while use the mesh key to get a new one by doing another login
	token, expires, err := s.issueSessionToken(req.NodeID, 0) // never expires for now, but we should fix this
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
const nodeExpiryCheckInterval = 1 * time.Minute

// nodeExpiry defines the duration after which a node is considered stale and eligible for expiration.
const nodeExpiry = 1 * time.Minute

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
