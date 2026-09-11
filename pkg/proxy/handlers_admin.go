package proxy

import (
	"net/http"
	"strings"

	"github.com/negrel/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

func (p *Proxy) adminEnabledHandler(rpc *jsonrpc.RPC) error {
	resp := struct {
		Enabled bool `json:"enabled"`
	}{
		Enabled: p.admin != nil,
	}
	return rpc.ReplyObject(&resp)
}

func (p *Proxy) adminEnableHandler(rpc *jsonrpc.RPC) error {
	req := struct {
		Token string `json:"token"`
	}{}
	if err := rpc.GetObject(&req); err != nil {
		return err
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "token is required")
	}

	admin := api.NewClient(p.mesh.AdminAddress(), req.Token).Admin()
	if _, err := admin.ListInvites(); err != nil {
		return jsonrpc.NewError(http.StatusPreconditionFailed, "invalid token")
	}

	err := p.cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Admin.Address = p.mesh.AdminAddress()
		cfg.Admin.Secret = req.Token
		return nil
	})
	if err != nil {
		return err
	}
	p.WithAdminController(admin)

	resp := struct {
		Enabled bool `json:"enabled"`
	}{
		Enabled: true,
	}
	return rpc.ReplyObject(&resp)
}

func (p *Proxy) adminCreateInvitedHandler(rpc *jsonrpc.RPC) error {
	assert.NotNil(p.admin)

	req := api.CreateInviteRequest{}
	if err := rpc.GetObject(&req); err != nil {
		return err
	}
	if req.MeshId == "" {
		req.MeshId = "default"
	}
	resp, err := p.admin.CreateInvite(req)
	if err != nil {
		return err
	}
	return rpc.ReplyObject(resp)
}

func (p *Proxy) adminListInvitesHandler(rpc *jsonrpc.RPC) error {
	assert.NotNil(p.admin)

	resp, err := p.admin.ListInvites()
	if err != nil {
		log.WithName("proxy").Warnf("Failed to list invites: %v", err)
		return err
	}
	return rpc.ReplyObject(resp)
}

func (p *Proxy) adminRevokeInviteHandler(rpc *jsonrpc.RPC) error {
	assert.NotNil(p.admin)

	id := rpc.PathVar("id")
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "invite id is required")
	}
	if err := p.admin.DeleteInvite(id); err != nil {
		return err
	}
	return rpc.ReplyObject(&api.DeleteInviteRequest{Invite: id})
}

func (p *Proxy) adminListNodesHandler(rpc *jsonrpc.RPC) error {
	assert.NotNil(p.admin)

	resp, err := p.admin.ListNodes()
	if err != nil {
		return err
	}
	return rpc.ReplyObject(resp)
}

func (p *Proxy) adminKickNodeHandler(rpc *jsonrpc.RPC) error {
	assert.NotNil(p.admin)

	id := rpc.PathVar("id")
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "node id is required")
	}
	if err := p.admin.KickPeer(id); err != nil {
		return err
	}
	return rpc.ReplyObject(&api.KickPeerResponse{NodeID: id})
}
