package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get("x-forwarded-for")
	if host == "" {
		host = r.RemoteAddr
	}
	user := "--"

	log.WithName("admin").Errorf("%s %s %d -- %s %s\n", host, user, http.StatusOK, r.Method, r.URL.Path)
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "404 Not Found: %s", r.URL.Path)
}

func (s *Server) logRequest(r *http.Request, user string, start time.Time) {
	host := r.Header.Get("x-forwarded-for")
	if host == "" {
		host = r.RemoteAddr
	}
	if user == "" {
		user = "--"
	}

	d := time.Since(start).Round(time.Millisecond)
	log.WithName("admin").Infof("%s %s %s %s %s\n", host, d.String(), user, r.Method, r.URL.Path)
}

func (s *Server) handle(fn func(*jsonrpc.RPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			s.logRequest(r, "--", start)
		}()

		if err := security.RequireSameOrigin(r); err != nil {
			security.RejectSameOrigin(w, err)
			return
		}

		ctx := jsonrpc.NewRPC(w, r)
		if err := fn(ctx); err != nil {
			if ce, ok := err.(*api.Error); ok {
				ctx.Error(ce.Code(), ce.Message())
			} else {
				ctx.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (s *Server) authenticated(fn func(*jsonrpc.RPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			s.logRequest(r, "--", start)
		}()

		if err := security.RequireSameOrigin(r); err != nil {
			security.RejectSameOrigin(w, err)
			return
		}

		user, code := s.auth.DoAuth(w, r)
		if code != http.StatusOK {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := jsonrpc.NewRPC(w, r).WithProps(jsonrpc.Properties{
			User:  user.User,
			Group: user.Group,
		})
		if err := fn(ctx); err != nil {
			if ce, ok := err.(*api.Error); ok {
				ctx.Error(ce.Code(), ce.Message())
			} else {
				ctx.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}
