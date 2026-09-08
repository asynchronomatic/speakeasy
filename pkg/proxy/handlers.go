package proxy

import (
	"net/http"
	"strings"
	"time"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

func (p *Proxy) logRequest(r *http.Request, user string, start time.Time) {
	host := r.Header.Get("x-forwarded-for")
	if host == "" {
		host = r.RemoteAddr
	}
	if user == "" {
		user = "--"
	}

	d := time.Since(start).Round(time.Millisecond)
	log.WithName("admin").Infof("%s %s %s %s %s\n", host, d.String(), user, r.Method, r.RequestURI)
}

func (p *Proxy) handle(fn func(*RPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			p.logRequest(r, "--", start)
		}()

		if err := api.RequireSameOrigin(r); err != nil {
			if ce, ok := err.(*api.Error); ok {
				http.Error(w, ce.Message(), ce.Code())
			} else {
				http.Error(w, err.Error(), http.StatusForbidden)
			}
			return
		}

		rpc := &RPC{w: w, r: r}
		if err := fn(rpc); err != nil {
			if ce, ok := err.(*api.Error); ok {
				rpc.Error(ce.Code(), ce.Message())
			} else {
				rpc.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (p *Proxy) authenticated(fn func(*RPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			p.logRequest(r, "--", start)
		}()

		if err := api.RequireSameOrigin(r); err != nil {
			api.RejectSameOrigin(w, err)
			return
		}

		if p.auth != nil {
			_, code := p.auth.DoAuth(w, r)
			if code != http.StatusOK {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		ctx := &RPC{w: w, r: r}
		if err := fn(ctx); err != nil {
			if ce, ok := err.(*api.Error); ok {
				ctx.Error(ce.Code(), ce.Message())
			} else {
				ctx.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (p *Proxy) authRequiredHandler(rpc *RPC) error {
	return rpc.ReplyObject(&struct {
		Required bool `json:"required"`
	}{Required: p.auth != nil})
}

func (p *Proxy) refreshWebsocketHandler(w http.ResponseWriter, r *http.Request) {
	if p.auth != nil {
		if r.Header.Get("Authorization") == "" {
			if tok := strings.TrimSpace(r.URL.Query().Get("access_token")); tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		if _, code := p.auth.DoAuth(w, r); code != http.StatusOK {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	p.notifier.Handle(w, r)
}

func (p *Proxy) withAdmin(fn func(*RPC) error) func(*RPC) error {
	return func(rpc *RPC) error {
		if p.admin == nil {
			return api.NewError(http.StatusServiceUnavailable, "admin not enabled")
		}
		return fn(rpc)
	}
}
