package proxy

import (
	"net/http"
	"strings"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

type themeResponse struct {
	Theme string `json:"theme"`
}

func (p *Proxy) themeGetHandler(rpc *jsonrpc.RPC) error {
	p.lock.RLock()
	defer p.lock.RUnlock()
	cfg, err := core.LoadConfigFile()
	if err != nil {
		return jsonrpc.NewError(http.StatusInternalServerError, err.Error())
	}
	return rpc.ReplyObject(&themeResponse{Theme: core.NormalizeTheme(cfg.Proxy.Theme)})
}

func (p *Proxy) themeSetHandler(rpc *jsonrpc.RPC) error {
	var req themeResponse
	if err := rpc.GetObject(&req); err != nil {
		return err
	}
	raw := strings.ToLower(strings.TrimSpace(req.Theme))
	theme := core.NormalizeTheme(raw)
	if raw != "" && theme != raw {
		return jsonrpc.NewError(http.StatusBadRequest, "invalid theme")
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := core.LoadConfigFile()
	if err != nil {
		return jsonrpc.NewError(http.StatusInternalServerError, err.Error())
	}
	cfg.Proxy.Theme = theme
	if err := core.SaveConfig(cfg); err != nil {
		return jsonrpc.NewError(http.StatusInternalServerError, err.Error())
	}
	return rpc.ReplyObject(&themeResponse{Theme: theme})
}
