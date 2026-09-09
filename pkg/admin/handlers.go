package admin

import (
	"net/http"
	"time"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	log.WithName("admin").Errorf("%s %s %d -- %s %s\n", security.ClientAddr(r), "--", http.StatusNotFound, security.RequestMethod(r), security.RequestPath(r))
	http.Error(w, "404 Not Found", http.StatusNotFound)
}

func (s *Server) logRequest(r *http.Request, user string, start time.Time) {
	if user == "" {
		user = "--"
	} else {
		user = security.SanitizeLog(user)
	}

	d := time.Since(start).Round(time.Millisecond)
	log.WithName("admin").Infof("%s %s %s %s %s\n", security.ClientAddr(r), d.String(), user, security.RequestMethod(r), security.RequestPath(r))
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
			security.WriteAuthError(w, code)
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
