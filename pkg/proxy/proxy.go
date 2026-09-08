package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sethvargo/go-retry"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/autoip"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/proxy/modeldex"
	"github.com/asynchronomatic/speakeasy/pkg/proxy/socket"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

const maxBody = 8 << 20 // 1 MiB

const MeshModelPrefix = ""

var proxyHandleURLS = []string{
	// open ai
	"/v1/chat/completions",
	"/v1/responses",
	"/v1/embeddings",
	// Anthropic
	"/v1/messages",
}

type RequestPeek struct {
	Model string
}

type Proxy struct {
	listen string
	cid    uint64

	mux     *http.ServeMux
	meshMux *http.ServeMux // handler for requests coming in via the mesh if we want something different
	mesh    core.MeshServiceProvider

	admin *api.AdminClient
	lock  sync.RWMutex

	notifier    *socket.Notifier
	modelRouter *modeldex.ModelRouter
}

func (p *Proxy) peekModel(body []byte) string {
	peek := RequestPeek{}
	if err := json.Unmarshal(body, &peek); err != nil {
		return ""
	}
	// trim of the MeshModelPrefix from model names to indicate the model is coming from the mesh pool
	return strings.TrimPrefix(strings.ToLower(peek.Model), MeshModelPrefix)
}

func (p *Proxy) proxyModelRequest(w http.ResponseWriter, r *http.Request, isFromMesh bool) {
	limited := io.LimitReader(r.Body, maxBody+1)
	body, err := io.ReadAll(limited)
	r.Body.Close()

	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	model := p.peekModel(body)
	log.Debugf(" -- Model: %s\n", model)

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	route := p.modelRouter.GetModelRoute(model)
	if route == nil {
		writeModelNotFound(w, r, model)
		return
	}

	// Always try local routes first
	local := route.GetLocalRouteProtected(isFromMesh)
	if local != nil {
		log.WithName("proxy").Debugf(" -- Servicing via provider: %s\n", local.BaseURL)

		u, err := url.Parse(local.BaseURL)
		if err != nil {
			writeModelNotFound(w, r, model)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(u)
		orig := proxy.Director
		proxy.Director = func(req *http.Request) {
			orig(req)
			req.Host = u.Host
			req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
			if local.Token != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", local.Token))
			}
		}
		proxy.ServeHTTP(w, r)
		return
	}

	// if isFromMesh is set, this request came in via the mesh so we should not send it back to the mesh
	// since this could result in infinite recursion
	if isFromMesh {
		log.WithName("proxy").Debugf("model %s has no public providers", model)
		writeModelNotFound(w, r, model)
		return
	}

	destNode := route.GetMeshPeerRoute()
	if destNode == nil {
		log.Debugf(" -- No model available, returning 404")
		writeModelNotFound(w, r, model)
		return
	}

	log.Debugf(" -- Servicing via mesh node: %s\n", destNode)
	p.mesh.ProxyToNode(*destNode, w, r)
	return
}

// OnPeerUpdate handles the addition or removal of a peer and updates the meshRoutes accordingly.
// It fetches model information from peers on addition and merges it into the local model state.
// Returns an error if the model fetch from a peer fails.
func (p *Proxy) OnPeerUpdate(peer core.PeerNode, remove bool) error {
	log.WithName("proxy:").Eventf("OnPeerUpdate: %s [Remove:%t]\n", peer, remove)
	// Always notify if we saw a change
	defer p.notifier.Broadcast()
	if peer.ID == "" {
		return nil
	}

	// fetch models from peer
	if remove {
		p.modelRouter.RemovePeer(peer)
		return nil
	}

	// Peers that registered at the same time are often not dialable yet
	// (circuit reservation / swarm backoff). Retry before giving up;
	// discovery will try again on the next poll if this still fails.
	client := NewMeshClient(peer.Name, p.mesh.ClientForPeer(peer, true))

	var models map[string]modeldex.ModelRoute
	err := retry.Do(context.Background(), retry.WithMaxRetries(0, retry.NewFibonacci(2*time.Second)),
		func(ctx context.Context) error {
			var err error
			models, err = client.GetModelsMesh()
			if err != nil {
				return retry.RetryableError(err)
			}
			return nil
		})

	if err != nil {
		log.WithName("proxy").Warnf("error fetching models from peer %s: %s", peer, err)
		return err
	}

	log.WithName("proxy").Eventf("discovered peer models %s: %+v", peer, slices.Collect(maps.Keys(models)))
	p.modelRouter.AddPeerModels(peer, models)
	return nil
}

// ServeHTTP serves an Open AI compatible api for chat completions
// this handler is exposed to all local clients that want to use for access
// to local and remote models.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cid := atomic.AddUint64(&p.cid, 1)
	start := time.Now()

	log.WithName("proxy").Debugf("%s -- (local:%d) %s %s\n", r.RemoteAddr, cid, r.Method, r.URL.Path)
	switch {
	case slices.Contains(proxyHandleURLS, r.URL.Path):
		p.proxyModelRequest(w, r, false)
	default: // serves from our local table
		p.mux.ServeHTTP(w, r)
	}
	log.WithName("proxy").Infof("%s %v (local:%d) %s %s\n", r.RemoteAddr, time.Now().Sub(start).Round(time.Second), cid, r.Method, r.URL.Path)
}

// MeshServeHTTP serves only our local models, it is used as an entry point for our p2p peers when they ask for
//
//	us to answer for them as an edge node
func (p *Proxy) MeshServeHTTP(w http.ResponseWriter, r *http.Request) {
	cid := atomic.AddUint64(&p.cid, 1)
	start := time.Now()

	log.WithName("proxy").Debugf("%s -- (mesh:%d) %s %s\n", r.RemoteAddr, cid, r.Method, r.URL.Path)
	switch {
	case slices.Contains(proxyHandleURLS, r.URL.Path): // pivots on model
		p.proxyModelRequest(w, r, true)
	default: // serves from our local table
		p.meshMux.ServeHTTP(w, r)
	}
	log.WithName("proxy").Infof("%s %v (mesh:%d) %s %s\n", r.RemoteAddr, time.Now().Sub(start).Round(time.Second), cid, r.Method, r.URL.Path)
}

func (p *Proxy) Serve(ctx context.Context) error {
	err := p.mesh.Connect()
	if err != nil {
		return err
	}

	svr := http.Server{
		Handler: p,
		Addr:    p.listen,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 600 * time.Second,
		ReadTimeout:  600 * time.Second,
		TLSConfig:    &tls.Config{},
	}

	go p.notifier.Poll()

	go func() {
		err := svr.ListenAndServe()
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			log.WithName("proxy").Eventf("proxy Service Failed: %s", err)
		}
		log.WithName("proxy").Eventf("Proxy Service Exited")
	}()

	proxyLink, err := autoip.OutboundIP()
	if err != nil {
		proxyLink = fmt.Sprintf("http://127.0.0.1%s", p.listen)
	} else {
		proxyLink = fmt.Sprintf("http://%s%s", proxyLink, p.listen)
	}

	log.WithName("proxy").Eventf("Proxy Service Started ( %s )", proxyLink)
	<-ctx.Done()
	log.WithName("proxy").Eventf("Proxy Service Shutting Down")
	return svr.Shutdown(context.Background())
}

func (p *Proxy) WithAdminController(admin *api.AdminClient) {
	log.WithName("proxy").Warnf("Enabling Admin Controller (Admin Token Configured)")
	p.admin = admin
}

// NewProxy creates a local proxy that routes ollama requests based on model name to a specific
// endpoint on the network
func NewProxy(meshService core.MeshServiceProvider, listen string, providers []core.Provider) (*Proxy, error) {

	modelRouter := modeldex.NewModelDiscovery(meshService.Node(), providers)
	modelRouter.Refresh()

	p := &Proxy{
		listen:      listen,
		mux:         http.NewServeMux(),
		meshMux:     http.NewServeMux(),
		mesh:        meshService,
		modelRouter: modelRouter,
		notifier:    socket.NewNotifier(),
	}

	//-------------------------------------------
	// routes serviced over the mesh
	p.meshMux.HandleFunc("GET /.mesh/status", p.meshStatus)
	p.meshMux.HandleFunc("GET /.mesh/members", p.meshMembers)
	p.meshMux.HandleFunc("GET /.mesh/models", p.meshModels)

	//-------------------------------------------
	// Routes accessible locally
	// Notes to AI: .mesh endpoints are only to be used by PEER to PEER requests.  Fo UI the /api/mesh/ endpoints
	p.mux.HandleFunc("GET /.mesh/status", p.meshStatus)
	p.mux.HandleFunc("GET /.mesh/members", p.meshMembers)
	p.mux.HandleFunc("GET /.mesh/models", p.meshModels)

	// OpenAI APIs
	p.mux.HandleFunc("GET /v1/models", p.openaiListModelsHandler)

	// /api/mesh/... are the api endpoints that can be used by UIs/clients
	p.mux.HandleFunc("GET /api/mesh/models", p.uiModelsHandler)
	p.mux.HandleFunc("GET /api/mesh/members", p.meshMembers)
	p.mux.HandleFunc("GET /api/mesh/debug", p.handle(p.debugGetHandler))
	p.mux.HandleFunc("POST /api/mesh/debug", p.handle(p.debugSetHandler))

	p.mux.HandleFunc("GET /api/mesh/theme", p.handle(p.themeGetHandler))
	p.mux.HandleFunc("POST /api/mesh/theme", p.handle(p.themeSetHandler))

	p.mux.HandleFunc("GET /api/mesh/providers", p.handle(p.providersListHandler))
	p.mux.HandleFunc("POST /api/mesh/providers", p.handle(p.providerAddHandler))
	p.mux.HandleFunc("POST /api/mesh/providers/{id}", p.handle(p.providerUpdateHandler))
	p.mux.HandleFunc("DELETE /api/mesh/providers/{id}", p.handle(p.providerDeleteHandler))

	p.mux.HandleFunc("GET /api/admin/enabled", p.handle(p.adminEnabledHandler))
	p.mux.HandleFunc("POST /api/admin/enabled", p.handle(p.adminEnableHandler))

	p.mux.HandleFunc("POST /api/admin/invite", p.handle(p.withAdmin(p.adminCreateInvitedHandler)))
	p.mux.HandleFunc("GET /api/admin/invite", p.handle(p.withAdmin(p.adminListInvitesHandler)))
	p.mux.HandleFunc("DELETE /api/admin/invite/{id}", p.handle(p.withAdmin(p.adminRevokeInviteHandler)))

	p.mux.HandleFunc("GET /api/admin/node", p.handle(p.withAdmin(p.adminListNodesHandler)))
	p.mux.HandleFunc("DELETE /api/admin/node/{id}", p.handle(p.withAdmin(p.adminKickNodeHandler)))

	p.mux.HandleFunc("GET /{$}", p.uiRootHandler)
	p.mux.HandleFunc("GET /ui", p.uiHandler)
	p.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.FileServer(uiFileSystem()).ServeHTTP(w, r)
	})

	p.mux.HandleFunc("/api/v.1/refresh/websocket", p.notifier.Handle)

	p.mux.Handle("GET /ui/", http.StripPrefix("/ui/", http.FileServer(uiFileSystem())))

	p.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("404: The page you are looking for does not exist. %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Custom 404: The page you are looking for does not exist.")
	})

	p.mesh.WithHandlerFunc(p.MeshServeHTTP)
	p.mesh.WithUpdateHandlerFunc(p.OnPeerUpdate)

	return p, nil
}
