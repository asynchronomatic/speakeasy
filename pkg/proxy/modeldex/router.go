package modeldex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/exp/maps"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"

	"github.com/ollama/ollama/api"
)

type ModelRouter struct {
	node       core.PeerNode
	providers  []config.Provider // configured providers for rescanning
	lock       sync.Mutex
	MeshModels map[string]ModelRoute // MeshModelswill be forwarded out
	httpClient *http.Client
}

// ListExportedModels returns a list of models exported from our instance
func (e *ModelRouter) ListExportedModels() []ModelRoute {
	e.lock.Lock()
	defer e.lock.Unlock()
	local := make([]ModelRoute, 0)

	for name := range e.MeshModels {
		model := e.MeshModels[name]
		if model.IsLocal() && !model.IsPrivate() {
			local = append(local, model)
		}
	}
	return local
}

// ListMeshModels lists all models discovered (local & peer)
func (e *ModelRouter) ListMeshModels() []ModelRoute {
	e.lock.Lock()
	defer e.lock.Unlock()
	return maps.Values(e.MeshModels)
}

func (e *ModelRouter) modelsFromWhitelist(provider *config.Provider) map[string]ModelRoute {
	modelTable := make(map[string]ModelRoute)

	for _, m := range provider.Models {
		route, ok := modelTable[m.Model]
		if !ok {
			route = MakeRoute(m.Model, m.Model, m.Capabilities)
		}

		route.ModifiedAt = time.Now()
		route.Owner = ""
		route.providers = append(route.providers, ModelProvider{
			ID:       provider.ID,
			Private:  m.Private,
			Provider: provider.Type,
			BaseURL:  provider.BaseURL,
			Token:    provider.Token,
		})
		modelTable[m.Model] = route
	}
	return modelTable
}

func (e *ModelRouter) ollamaFetchModels(provider *config.Provider) (map[string]ModelRoute, error) {
	models := make(map[string]ModelRoute)

	if _, err := config.ParseProviderURL(provider.BaseURL, true); err != nil {
		log.WithName("mdex").Errorf("failed to parse provider URL: %v", err)
		return nil, err
	}

	u, err := url.Parse(provider.BaseURL)
	if err != nil {
		return nil, err
	}

	client := api.NewClient(u, e.httpClient)
	ctx := context.Background()

	resp, err := client.List(ctx) // GET /api/tags
	if err != nil {
		return nil, fmt.Errorf("failed to list running models: %v", err)
	}

	properties := make(map[string]api.ListModelResponse)
	for k := range resp.Models {
		m := resp.Models[k]
		properties[m.Name] = m
	}

	// this is wrong whitelist should control whether the model is added or not
	if provider.Discovery == "whitelist" {
		models = e.modelsFromWhitelist(provider)
	} else {
		running, err := client.ListRunning(ctx) // GET /api/ps
		if err != nil {
			return nil, fmt.Errorf("failed to list running models: %v", err)
		}

		for _, m := range running.Models {
			route, ok := models[m.Model]
			if !ok {
				route = MakeRoute(m.Model, m.Model, []string{})
				route.Owner = "ollama"
			}

			route.providers = append(route.providers, ModelProvider{
				ID:       provider.ID,
				Private:  provider.Private,
				Provider: provider.Type,
				BaseURL:  provider.BaseURL,
				Token:    provider.Token,
			})
			route.Owner = "ollama"
			route.ModifiedAt = time.Now()
			route.ContextLength = m.Details.ContextLength
			models[m.Model] = route
		}
	}

	for k := range models {
		p, ok := properties[k]
		if !ok {
			delete(models, k)
			continue
		}

		m := models[k]
		if m.ContextLength == 0 {
			m.ContextLength = p.Details.ContextLength
		}

		for _, c := range p.Capabilities {
			m.Capabilities = append(m.Capabilities, string(c))
		}
		models[k] = m
	}

	return models, nil
}

func (e *ModelRouter) openaiFetchModels(provider *config.Provider) (map[string]ModelRoute, error) {
	if _, err := config.ParseProviderURL(provider.BaseURL, true); err != nil {
		return nil, err
	}
	client := jsonrpc.NewClient(provider.BaseURL, provider.Token).WithDoer(e.httpClient)

	whitelist := make(map[string]ModelRoute)
	if provider.Discovery == "whitelist" {
		whitelist = e.modelsFromWhitelist(provider)
	}

	resp := OpenaiModelList{}
	err := client.Get("/v1/models", &resp)
	if err != nil {
		return nil, err
	}

	models := make(map[string]ModelRoute)
	for _, m := range resp.Data {
		w, ok := whitelist[m.ID]
		if !ok {
			continue
		}

		route, ok := models[m.ID]
		if !ok {
			route = MakeRoute(m.ID, m.ID, []string{})
			route.ContextLength = m.ContextLength
			route.Capabilities = w.Capabilities
		}
		route.providers = append(route.providers, ModelProvider{
			ID:       provider.ID,
			Private:  provider.Private,
			Provider: provider.Type,
			BaseURL:  provider.BaseURL,
			Token:    provider.Token,
		})
		route.Owner = m.OwnedBy
		route.ModifiedAt = time.Unix(m.Created, 0)
		models[m.ID] = route
	}
	return models, nil
}

func (e *ModelRouter) testFetchModels(provider *config.Provider) (map[string]ModelRoute, error) {
	return e.modelsFromWhitelist(provider), nil
}

func (e *ModelRouter) ListModels() []string {
	models := make([]string, 0)
	for _, m := range e.MeshModels {
		models = append(models, m.Name)
	}
	return models
}

// fixme.... an update can add and remove a model
func (e *ModelRouter) UpdatePeerModels(node core.PeerNode, peerModels map[string]ModelRoute) {
	e.lock.Lock()
	defer e.lock.Unlock()
	// remove any models that are missing
	for k, model := range e.MeshModels {
		if _, ok := model.peers[node.ID]; !ok {
			continue
		}
		if _, ok := peerModels[k]; !ok {
			model.RemovePeer(node)
		}
		if !model.IsAvailable() {
			delete(e.MeshModels, k)
		}
	}

	// add or update existing
	for name := range peerModels {
		route, ok := e.MeshModels[name]
		if !ok {
			log.Debugf("adding peer model %s: %+v", name, peerModels[name])
			route = peerModels[name]
		}

		route.AddPeer(node)
		e.MeshModels[name] = route
	}
}

func (e *ModelRouter) RemovePeer(node core.PeerNode) {
	e.lock.Lock()
	defer e.lock.Unlock()

	for k, model := range e.MeshModels {
		model.RemovePeer(node)

		if !model.IsAvailable() {
			delete(e.MeshModels, k)
		}
	}
}

func (e *ModelRouter) RemoveProvider(provider config.Provider) {
	e.lock.Lock()
	defer e.lock.Unlock()

	for k, model := range e.MeshModels {
		changed := false
		for i, p := range model.providers {
			if p.ID == provider.ID {
				model.providers = append(model.providers[:i], model.providers[i+1:]...)
				delete(e.MeshModels, k)
				model.RemovePeer(e.node)
				changed = true
			}
		}

		if changed {
			e.MeshModels[k] = model
		}

		if !model.IsAvailable() {
			delete(e.MeshModels, k)
		}
	}
}

func (e *ModelRouter) AddProvider(provider config.Provider) error {
	return e.handleProviderCreate(provider)
}

func (e *ModelRouter) handleProviderCreate(provider config.Provider) error {
	var err error
	var routes map[string]ModelRoute

	switch provider.Type {
	case "ollama":
		routes, err = e.ollamaFetchModels(&provider)
		if err != nil {
			return err
		}
	case "openai":
		routes, err = e.openaiFetchModels(&provider)
		if err != nil {
			return err
		}

	case "test":
		routes, err = e.testFetchModels(&provider)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported")
	}

	e.UpdatePeerModels(e.node, routes)
	return nil
}

func (e *ModelRouter) Refresh() {
	for _, provider := range e.providers {
		err := e.handleProviderCreate(provider)
		if err != nil {
			log.Errorf("%s: %v", provider.ID, err)
		}
	}
}

func (e *ModelRouter) GetModelRoute(model string) *ModelRoute {
	e.lock.Lock()
	defer e.lock.Unlock()
	if route, ok := e.MeshModels[model]; ok {
		return &route
	}
	return nil
}

func NewModelDiscovery(node core.PeerNode, providers []config.Provider, httpClient *http.Client) *ModelRouter {
	if httpClient == nil {
		httpClient = config.NewProviderHTTPClient(false)
	}
	return &ModelRouter{
		node:       node,
		providers:  providers,
		MeshModels: make(map[string]ModelRoute),
		httpClient: httpClient,
	}
}
