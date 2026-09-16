package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
	"github.com/asynchronomatic/speakeasy/pkg/admin/magiclink"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/secrets"
)

var BuildVersion string

// nodeExpiryCheckInterval defines the interval for checking and expiring stale node registrations.
const nodeExpiryCheckInterval = 1 * time.Minute

// nodeExpiry defines the duration after which a node is considered stale and eligible for expiration.
const nodeExpiry = 5 * time.Minute

type Server struct {
	mainAddress  string
	adminKey     string
	relayAddress []string
	httpServer   *http.Server
	lock         sync.Mutex
	db           *jsonkv.Store
	auth         auth.Provider

	//baseUrl  string
	advertiseURL string
	magicKey     magiclink.EncryptionKey

	nodeStore   *MeshNodeStore
	inviteStore *InviteStore
}

func adminDBPath() string {
	if p := strings.TrimSpace(os.Getenv("ADMIN_DB_PATH")); p != "" {
		return p
	}
	return "admin.jkv"
}

func (s *Server) runExpireNodes(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(nodeExpiryCheckInterval):
			s.nodeStore.ExpireStaleNodes(nodeExpiry)
		}
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// login
	mux.HandleFunc("POST /api/v1/login", s.handle(s.apiNodeLogin))

	// authenticated:
	mux.HandleFunc("GET /api/v1/relay", s.authenticated(s.apiRelayGet))
	// @deprecate or fix this endpoint
	mux.HandleFunc("POST /api/v1/authorize", s.authenticated(s.apiNodeAuthorize))
	mux.HandleFunc("POST /api/v1/nodes", s.authenticated(s.apiNodeRegister))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.authenticated(s.apiNodeUnregister))
	mux.HandleFunc("POST /api/v1/nodes/{id}", s.authenticated(s.apiNodeRefresh))
	mux.HandleFunc("GET /api/v1/nodes", s.authenticated(s.apiNodeList))

	// admin: only
	mux.HandleFunc("GET /api/v1/admin/nodes", s.authenticated(jsonrpc.AsAdmin(s.adminListNodes)))
	mux.HandleFunc("DELETE /api/v1/admin/nodes/{id}", s.authenticated(jsonrpc.AsAdmin(s.adminDeleteNode)))
	mux.HandleFunc("POST /api/v1/admin/nodes/{id}", s.authenticated(jsonrpc.AsAdmin(s.adminDeleteNode)))

	mux.HandleFunc("POST /api/v1/admin/invite", s.authenticated(jsonrpc.AsAdmin(s.adminCreateInviteLink)))
	mux.HandleFunc("GET /api/v1/admin/invite", s.authenticated(jsonrpc.AsAdmin(s.adminListInviteLinks)))
	mux.HandleFunc("DELETE /api/v1/admin/invite/{id}", s.authenticated(jsonrpc.AsAdmin(s.adminDeleteInviteLink)))
	mux.HandleFunc("DELETE /api/v1/admin/peer/{id}", s.authenticated(jsonrpc.AsAdmin(s.adminKickPeer)))

	// public: redeem is public since it's getting a magic link
	mux.HandleFunc("POST /api/v1/redeem/{id}", s.handle(s.adminRedeemInviteLink))

	mux.HandleFunc("/", notFoundHandler)
	return secrets.Handler(mux)
}

func (s *Server) Listen() error {
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
	return s.nodeStore.acl
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
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
			_ = resp.Body.Close()
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

	a := auth.NewTokenAuth()
	err = a.AddToken(adminKey, "admin", AdminGroup)
	if err != nil {
		return nil, err
	}

	kv, err := jsonkv.Open(adminDBPath())
	if err != nil {
		return nil, err
	}

	nodeStore, err := NewMeshNodeStore("default", kv, NewAllowList())
	if err != nil {
		_ = kv.Close()
		return nil, err
	}

	s := &Server{
		mainAddress: listenAddress,
		adminKey:    adminKey,
		db:          kv,
		auth:        a,
		magicKey:    magicKey,
		inviteStore: NewInviteStore("default", kv),
		nodeStore:   nodeStore,
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
