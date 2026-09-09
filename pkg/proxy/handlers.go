package proxy

import (
	"net/http"
	"time"

	"github.com/negrel/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func (p *Proxy) logRequest(r *http.Request, user string, start time.Time) {
	host := security.ClientAddr(r)
	if user == "" {
		user = "--"
	} else {
		user = security.SanitizeLog(user)
	}

	d := time.Since(start).Round(time.Millisecond)
	log.WithName("admin").Infof("%s %s %s %s %s\n", host, d.String(), user, security.RequestMethod(r), security.RequestPath(r))
}

func (p *Proxy) handle(fn func(*jsonrpc.RPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			p.logRequest(r, "--", start)
		}()

		if err := security.RequireSameOrigin(r); err != nil {
			if ce, ok := err.(*api.Error); ok {
				http.Error(w, ce.Message(), ce.Code())
			} else {
				http.Error(w, err.Error(), http.StatusForbidden)
			}
			return
		}

		rpc := jsonrpc.NewRPC(w, r)
		if err := fn(rpc); err != nil {
			if ce, ok := err.(*api.Error); ok {
				rpc.Error(ce.Code(), ce.Message())
			} else {
				rpc.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (p *Proxy) authenticated(fn func(*jsonrpc.RPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assert.NotNil(p.auth)

		start := time.Now()
		defer func() {
			p.logRequest(r, "--", start)
		}()

		if err := security.RequireSameOrigin(r); err != nil {
			security.RejectSameOrigin(w, err)
			return
		}

		user, code := p.auth.DoAuth(w, r)
		if code != http.StatusOK {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		rpc := jsonrpc.NewRPC(w, r).WithProps(jsonrpc.Properties{
			User:  user.User,
			Group: user.Group,
		})

		if err := fn(rpc); err != nil {
			if ce, ok := err.(*jsonrpc.Error); ok {
				_ = rpc.Error(ce.Code(), ce.Message())
			} else {
				_ = rpc.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (p *Proxy) authRequiredHandler(rpc *jsonrpc.RPC) error {
	return rpc.ReplyObject(&struct {
		Required bool `json:"required"`
	}{Required: p.auth != nil})
}

func (p *Proxy) refreshWebsocketHandler(w http.ResponseWriter, r *http.Request) {
	if p.auth != nil {
		if !p.consumeWSTicket(r) {
			if _, code := p.auth.DoAuth(w, r); code != http.StatusOK {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
	}
	p.notifier.Handle(w, r)
}

func (p *Proxy) withAdmin(fn func(*jsonrpc.RPC) error) func(*jsonrpc.RPC) error {
	return func(rpc *jsonrpc.RPC) error {
		if p.admin == nil {
			return jsonrpc.NewError(http.StatusServiceUnavailable, "admin not enabled")
		}
		return fn(rpc)
	}
}
