package admin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
	"github.com/asynchronomatic/speakeasy/pkg/admin/magiclink"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
	"github.com/asynchronomatic/speakeasy/pkg/log"

	"github.com/asynchronomatic/speakeasy/api"
)

var BuildVersion string

type NodeReference struct {
	Node     api.Node
	Token    string
	LastPing time.Time
}

type Server struct {
	mainAddress  string
	adminKey     string
	relayAddress []string
	httpServer   *http.Server
	lock         sync.Mutex
	lastUpdate   time.Time
	logicalTime  uint64
	nodes        map[string]*NodeReference
	acl          *AllowList
	kv           *jsonkv.Store

	auth auth.Provider

	//baseUrl  string
	advertiseURL string
	magicKey     magiclink.EncryptionKey
}

func OutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected local addr type: %T", conn.LocalAddr())
	}
	return udpAddr.IP.String(), nil
}

// only allow the admin user in
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
			rejectSameOrigin(w, err)
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

func rejectSameOrigin(w http.ResponseWriter, err error) {
	if ce, ok := err.(*api.Error); ok {
		http.Error(w, ce.Message(), ce.Code())
		return
	}
	http.Error(w, err.Error(), http.StatusForbidden)
}

func (s *Server) authenticated(fn func(*JsonRPC) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			s.logRequest(r, "--", start)
		}()

		if err := api.RequireSameOrigin(r); err != nil {
			rejectSameOrigin(w, err)
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

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/login", s.handle(s.apiNodeLogin))

	mux.HandleFunc("GET /api/v1/relay", s.authenticated(s.apiRelayGet))
	mux.HandleFunc("POST /api/v1/authorize", s.authenticated(s.apiNodeAuthorize))
	mux.HandleFunc("POST /api/v1/nodes", s.authenticated(s.apiNodeRegister))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.authenticated(s.apiNodeUnregister))
	mux.HandleFunc("POST /api/v1/nodes/{id}", s.authenticated(s.apiNodeRefresh))
	mux.HandleFunc("GET /api/v1/nodes", s.authenticated(s.apiNodeList))

	mux.HandleFunc("GET /api/v1/admin/nodes", s.authenticated(s.asAdmin(s.adminListNodes)))
	mux.HandleFunc("DELETE /api/v1/admin/nodes/{id}", s.authenticated(s.asAdmin(s.adminDeleteNode)))
	mux.HandleFunc("POST /api/v1/admin/nodes/{id}", s.authenticated(s.asAdmin(s.adminDeleteNode)))

	mux.HandleFunc("POST /api/v1/admin/invite", s.authenticated(s.asAdmin(s.adminCreateInviteLink)))
	mux.HandleFunc("GET /api/v1/admin/invite", s.authenticated(s.asAdmin(s.adminListInviteLinks)))
	mux.HandleFunc("DELETE /api/v1/admin/invite/{id}", s.authenticated(s.asAdmin(s.adminDeleteInviteLink)))
	mux.HandleFunc("DELETE /api/v1/admin/peer/{id}", s.authenticated(s.asAdmin(s.adminKickPeer)))

	// redeem is public since it's getting a magic link
	mux.HandleFunc("POST /api/v1/redeem/{id}", s.handle(s.adminRedeemInviteLink))

	mux.HandleFunc("/", notFoundHandler)
	return mux
}

func (s *Server) Listen() error {
	// expire old nodes

	return s.Serve(context.Background())
}

func (s *Server) Serve(ctx context.Context) error {
	log.Eventf("Starting On  on %s\n", s.mainAddress)

	s.httpServer = &http.Server{
		Addr:              s.mainAddress,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	go func() {
		s.runExpireNodes(ctx)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		err := <-errCh
		_ = s.Close()
		return err
	case err := <-errCh:
		if err != nil {
			log.Errorf("error serving admin: %s", err)
		}
		_ = s.Close()
		return err
	}
}

func (s *Server) GetAllowList() *AllowList {
	return s.acl
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	return s.kv.Close()
}

func (s *Server) WithRelayAddresses(advertiseAddresses []string) {
	s.relayAddress = advertiseAddresses
}

func (s *Server) Wait(ctx context.Context) error {
	url := "http://localhost" + s.mainAddress
	if url == "" {
		return fmt.Errorf("public address not set")
	}

	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("server at %s not ready: %w", url, ctx.Err())
		case <-ticker.C:
		}
	}
}

func adminDBPath() string {
	if p := strings.TrimSpace(os.Getenv("ADMIN_DB_PATH")); p != "" {
		return p
	}
	return "admin.jkv"
}

func (s *Server) WithAdvertiseURL(url string) *Server {
	s.advertiseURL = url
	return s
}

func NewServer(listenAddress, adminKey string) (*Server, error) {
	log.Printf("Build Version: %s\n", BuildVersion)

	magicKey, err := magiclink.GenerateKey()
	if err != nil {
		return nil, err
	}

	acl, _ := NewAllowList("allow.list")

	kv, err := jsonkv.Open(adminDBPath())
	if err != nil {
		return nil, err
	}

	a := auth.NewTokenAuth()
	a.AddUser("admin", AdminGroup, adminKey)

	s := &Server{
		mainAddress: listenAddress,
		adminKey:    adminKey,
		nodes:       make(map[string]*NodeReference),
		acl:         acl,
		kv:          kv,
		auth:        a,
		magicKey:    magicKey,
	}
	a.SetSessionAuth(func(token string) (*auth.Properties, bool) {
		props, err := s.authenticateSessionToken(token)
		if err != nil {
			return nil, false
		}
		return props, true
	})

	return s, nil
}
