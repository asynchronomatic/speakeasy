package proxy

import (
	"net/http"
	"strings"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
)

type providersListResponse struct {
	Providers []core.Provider `json:"providers"`
}

func providerIndex(providers []core.Provider, id string) int {
	for i := range providers {
		if providers[i].ID == id {
			return i
		}
	}
	return -1
}

func validateProvider(prov *core.Provider) error {
	prov.ID = strings.TrimSpace(prov.ID)
	prov.Type = strings.TrimSpace(prov.Type)
	prov.BaseURL = strings.TrimSpace(prov.BaseURL)
	if prov.ID == "" {
		return api.NewError(http.StatusBadRequest, "provider id is required")
	}
	if prov.Type == "" {
		return api.NewError(http.StatusBadRequest, "provider type is required")
	}
	if prov.BaseURL == "" {
		return api.NewError(http.StatusBadRequest, "provider base_url is required")
	}
	return nil
}

func (p *Proxy) loadProvidersConfig() (*core.Config, error) {
	cfg, err := core.LoadConfigFile()
	if err != nil {
		return nil, api.NewError(http.StatusInternalServerError, err.Error())
	}
	if cfg.Providers == nil {
		cfg.Providers = []core.Provider{}
	}
	return cfg, nil
}

func (p *Proxy) saveProvidersConfig(cfg *core.Config) error {
	if err := core.SaveConfig(cfg); err != nil {
		return api.NewError(http.StatusInternalServerError, err.Error())
	}
	return nil
}

func (p *Proxy) providersListHandler(rpc *RPC) error {
	p.lock.RLock()
	defer p.lock.RUnlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	return rpc.ReplyObject(&providersListResponse{Providers: cfg.Providers})
}

func (p *Proxy) providerAddHandler(rpc *RPC) error {
	var prov core.Provider
	if err := rpc.GetObject(&prov); err != nil {
		return err
	}
	if err := validateProvider(&prov); err != nil {
		return err
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	if providerIndex(cfg.Providers, prov.ID) >= 0 {
		return api.NewError(http.StatusConflict, "provider id already exists")
	}
	cfg.Providers = append(cfg.Providers, prov)
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}
	p.notifier.Broadcast() // notify ui of update
	return rpc.ReplyObject(&prov)
}

func (p *Proxy) providerUpdateHandler(rpc *RPC) error {
	id := strings.TrimSpace(rpc.PathVar("id"))
	if id == "" {
		return api.NewError(http.StatusBadRequest, "provider id is required")
	}
	var prov core.Provider
	if err := rpc.GetObject(&prov); err != nil {
		return err
	}
	prov.ID = id
	if err := validateProvider(&prov); err != nil {
		return err
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	i := providerIndex(cfg.Providers, id)
	if i < 0 {
		return api.NewError(http.StatusNotFound, "provider not found")
	}
	cfg.Providers[i] = prov
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}
	p.notifier.Broadcast() // notify ui of update
	return rpc.ReplyObject(&prov)
}

func (p *Proxy) providerDeleteHandler(rpc *RPC) error {
	id := strings.TrimSpace(rpc.PathVar("id"))
	if id == "" {
		return api.NewError(http.StatusBadRequest, "provider id is required")
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	i := providerIndex(cfg.Providers, id)
	if i < 0 {
		return api.NewError(http.StatusNotFound, "provider not found")
	}
	cfg.Providers = append(cfg.Providers[:i], cfg.Providers[i+1:]...)
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}

	p.notifier.Broadcast() // notify ui of update
	return rpc.ReplyObject(map[string]string{"id": id})
}
