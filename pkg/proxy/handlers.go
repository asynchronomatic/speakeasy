package proxy

import (
	"net/http"
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

func (p *Proxy) withAdmin(fn func(*RPC) error) func(*RPC) error {
	return func(rpc *RPC) error {
		if p.admin == nil {
			return api.NewError(http.StatusServiceUnavailable, "admin not enabled")
		}
		return fn(rpc)
	}
}
