package proxy

import (
	"net/http"
	"strings"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

type themeResponse struct {
	Theme string `json:"theme"`
}

func (p *Proxy) themeGetHandler(rpc *jsonrpc.RPC) error {
	var resp themeResponse

	err := p.cm.ReadConfig(func(config *config.Config) error {
		resp = themeResponse{Theme: config.Proxy.Theme}
		return nil
	})
	if err != nil {
		return err
	}
	return rpc.ReplyObject(&resp)
}

func (p *Proxy) themeSetHandler(rpc *jsonrpc.RPC) error {
	var req themeResponse
	if err := rpc.GetObject(&req); err != nil {
		return err
	}
	raw := strings.ToLower(strings.TrimSpace(req.Theme))
	theme := config.NormalizeTheme(raw)
	if raw != "" && theme != raw {
		return jsonrpc.NewError(http.StatusBadRequest, "invalid theme")
	}

	err := p.cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.Theme = theme
		return nil
	})
	if err != nil {
		return err
	}

	p.notifier.Broadcast()
	return rpc.ReplyObject(&themeResponse{Theme: theme})
}
