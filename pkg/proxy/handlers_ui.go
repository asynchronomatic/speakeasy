package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ollama/ollama/types/model"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/web"
)

type UIModelProvider = core.PeerNode

type UIModel struct {
	Name          string            `json:"name"`
	Model         string            `json:"model"`
	Private       bool              `json:"private"`
	Owner         string            `json:"owner"`
	ModifiedAt    time.Time         `json:"modified_at"`
	ContextLength int               `json:"context_length"`
	Capabilities  []string          `json:"capabilities,omitempty"`
	Providers     []UIModelProvider `json:"providers"`
}

func capabilityStrings(caps []model.Capability) []string {
	out := make([]string, 0, len(caps))
	seen := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		s := strings.TrimSpace(string(c))
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

type UIModelsResponse struct {
	Models []UIModel `json:"models"`
}

func findWebDir() (string, bool) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for {
			candidates = append(candidates, filepath.Join(dir, "web"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, d := range candidates {
		if st, err := os.Stat(filepath.Join(d, "index.html")); err == nil && !st.IsDir() {
			return d, true
		}
	}
	return "", false
}

func webDir() string {
	if d, ok := findWebDir(); ok {
		return d
	}
	return "web"
}

func uiFileSystem() http.FileSystem {
	if dir, ok := findWebDir(); ok {
		log.Debugf("serving UI from %s", dir)
		return http.Dir(dir)
	}
	log.Debugf("serving UI from embedded web/")
	return http.FS(web.FS)
}

func (p *Proxy) uiRootHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
}

func (p *Proxy) uiHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
}

func (p *Proxy) uiModelsHandler(w http.ResponseWriter, r *http.Request) {
	//peers, _ := p.mesh.GetPeerMap()

	p.lock.RLock()
	defer p.lock.RUnlock()

	models := p.modelRouter.ListMeshModels()

	resp := UIModelsResponse{
		Models: make([]UIModel, 0, len(models)),
	}

	for _, route := range models {
		m := UIModel{
			Name:          route.Name,
			Model:         route.Model,
			Private:       route.IsPrivate(),
			ContextLength: route.ContextLength,
			ModifiedAt:    route.ModifiedAt,
			Owner:         route.Owner,
			Capabilities:  route.Capabilities,
			Providers:     route.GetPeers(),
		}
		resp.Models = append(resp.Models, m)
	}

	sort.Slice(resp.Models, func(i, j int) bool {
		return resp.Models[i].Name < resp.Models[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&resp)
}

func shortPeer(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "…"
}

func (p *Proxy) debugSetHandler(rpc *RPC) error {
	req := struct {
		DebugEnabled bool `json:"debugEnabled"`
	}{}

	if err := rpc.GetObject(&req); err != nil {
		return err
	}

	if req.DebugEnabled {
		log.Default.SetLevel(log.LogAll)
	} else {
		log.Default.SetLevel(log.LogNormal)
	}

	return rpc.ReplyObject(&req)

}

func (p *Proxy) debugGetHandler(rpc *RPC) error {
	req := struct {
		DebugEnabled bool `json:"debugEnabled"`
	}{
		DebugEnabled: log.Default.GetLevel() == log.LogAll,
	}

	return rpc.ReplyObject(&req)
}
