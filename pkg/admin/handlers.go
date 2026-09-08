package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/log"
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
	log.WithName("admin").Infof("%s %s %s %s %s\n", host, d.String(), user, r.Method, r.RequestURI)
}

func (s *Server) asAdmin(fn func(*JsonRPC) error) func(*JsonRPC) error {
	return func(ctx *JsonRPC) error {
		if ctx.Group() != AdminGroup {
			return api.NewError(http.StatusUnauthorized, "not authorized")
		}
		return fn(ctx)
	}
}

func (s *Server) handle(fn func(*JsonRPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			s.logRequest(r, "--", start)
		}()

		if err := api.RequireSameOrigin(r); err != nil {
			api.RejectSameOrigin(w, err)
			return
		}

		ctx := &JsonRPC{w: w, r: r, user: nil}
		if err := fn(ctx); err != nil {
			if ce, ok := err.(*api.Error); ok {
				ctx.Error(ce.Code(), ce.Message())
			} else {
				ctx.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (s *Server) authenticated(fn func(*JsonRPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			s.logRequest(r, "--", start)
		}()

		if err := api.RequireSameOrigin(r); err != nil {
			api.RejectSameOrigin(w, err)
			return
		}

		user, code := s.auth.DoAuth(w, r)
		if code != http.StatusOK {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := &JsonRPC{w: w, r: r, user: user}
		if err := fn(ctx); err != nil {
			if ce, ok := err.(*api.Error); ok {
				ctx.Error(ce.Code(), ce.Message())
			} else {
				ctx.Error(http.StatusInternalServerError, err.Error())
			}
		}
	}
}
