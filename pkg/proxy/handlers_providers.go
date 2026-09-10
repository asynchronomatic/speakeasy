package proxy

import (
	"net/http"
	"strings"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/proxy/auth"
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

func (p *Proxy) validateProvider(prov *core.Provider) error {
	prov.ID = strings.TrimSpace(prov.ID)
	prov.Type = strings.TrimSpace(prov.Type)
	prov.BaseURL = strings.TrimSpace(prov.BaseURL)
	if prov.ID == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "provider id is required")
	}
	if prov.Type == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "provider type is required")
	}
	if prov.BaseURL == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "provider base_url is required")
	}
	if _, err := core.ParseProviderURL(prov.BaseURL, p.allowPrivate); err != nil {
		return jsonrpc.NewError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func (p *Proxy) loadProvidersConfig() (*core.Config, error) {
	cfg, err := core.LoadConfigFile()
	if err != nil {
		return nil, jsonrpc.NewError(http.StatusInternalServerError, err.Error())
	}
	if cfg.Providers == nil {
		cfg.Providers = []core.Provider{}
	}
	return cfg, nil
}

func (p *Proxy) saveProvidersConfig(cfg *core.Config) error {
	if err := core.SaveConfig(cfg); err != nil {
		return jsonrpc.NewError(http.StatusInternalServerError, err.Error())
	}
	return nil
}

func providersWithoutTokens(src []core.Provider) []core.Provider {
	out := make([]core.Provider, len(src))
	copy(out, src)
	for i := range out {
		out[i].Token = ""
	}
	return out
}

func providerWithoutToken(prov core.Provider) core.Provider {
	prov.Token = ""
	return prov
}

func keepProviderToken(submitted, existing string) string {
	submitted = strings.TrimSpace(submitted)
	if submitted == "" || submitted == "*" {
		return existing
	}
	return submitted
}

func (p *Proxy) providersListHandler(rpc *jsonrpc.RPC) error {
	p.lock.RLock()
	defer p.lock.RUnlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	return rpc.ReplyObject(&providersListResponse{Providers: providersWithoutTokens(cfg.Providers)})
}

func (p *Proxy) providerAddHandler(rpc *jsonrpc.RPC) error {
	var prov core.Provider
	if err := rpc.GetObject(&prov); err != nil {
		return err
	}
	if err := p.validateProvider(&prov); err != nil {
		return err
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	if providerIndex(cfg.Providers, prov.ID) >= 0 {
		return jsonrpc.NewError(http.StatusConflict, "provider id already exists")
	}
	cfg.Providers = append(cfg.Providers, prov)
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}
	p.notifier.Broadcast() // notify ui of update
	return rpc.ReplyObject(providerWithoutToken(prov))
}

func (p *Proxy) providerUpdateHandler(rpc *jsonrpc.RPC) error {
	id := strings.TrimSpace(rpc.PathVar("id"))
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "provider id is required")
	}
	var prov core.Provider
	if err := rpc.GetObject(&prov); err != nil {
		return err
	}
	prov.ID = id
	if err := p.validateProvider(&prov); err != nil {
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
		return jsonrpc.NewError(http.StatusNotFound, "provider not found")
	}
	prov.Token = keepProviderToken(prov.Token, cfg.Providers[i].Token)
	cfg.Providers[i] = prov
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}
	p.notifier.Broadcast() // notify ui of update
	return rpc.ReplyObject(providerWithoutToken(prov))
}

func (p *Proxy) providerDeleteHandler(rpc *jsonrpc.RPC) error {
	id := strings.TrimSpace(rpc.PathVar("id"))
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "provider id is required")
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	i := providerIndex(cfg.Providers, id)
	if i < 0 {
		return jsonrpc.NewError(http.StatusNotFound, "provider not found")
	}
	cfg.Providers = append(cfg.Providers[:i], cfg.Providers[i+1:]...)
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}

	p.notifier.Broadcast() // notify ui of update
	return rpc.ReplyObject(map[string]string{"id": id})
}

type inferenceToken struct {
	Name      string    `json:"name"`
	Token     string    `json:"token,omitempty"`
	Secret    string    `json:"secret,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type inferenceTokenCreateRequest struct {
	Insecure bool            `json:"insecure"` // if insecure is set Name and Expiry must be empty
	Token    *inferenceToken `json:"token"`
}

type inferenceTokenList struct {
	Insecure bool             `json:"insecure"`
	Tokens   []inferenceToken `json:"tokens"`
}

func inferenceTokenIndex(tokens []core.InferenceToken, token string) int {
	for i := range tokens {
		parsed, _, err := auth.ParseInferenceToken(tokens[i].Token)
		if err != nil {
			continue
		}
		if parsed == token {
			return i
		}
	}
	return -1
}

func Truncate(s string) string {
	parts := strings.Split(s, "$")
	return parts[0]
}

func publicInferenceTokens(cfg *core.Config) inferenceTokenList {
	src := cfg.Proxy.InferenceTokens.Tokens
	out := make([]inferenceToken, 0, len(src))
	for _, tok := range src {
		out = append(out, inferenceToken{
			Name:      tok.Name,
			Token:     Truncate(tok.Token),
			CreatedAt: tok.Created,
		})
	}
	return inferenceTokenList{
		Insecure: cfg.Proxy.InferenceTokens.Insecure,
		Tokens:   out,
	}
}

func (p *Proxy) inferenceTokenCreate(rpc *jsonrpc.RPC) error {
	var req inferenceTokenCreateRequest
	if err := rpc.GetObject(&req); err != nil {
		return err
	}

	if req.Token == nil {
		p.inferenceAuth.SetInsecure(req.Insecure)
		// persist to config

		// this needs protection via config updater lock
		cfg, err := p.loadProvidersConfig()
		if err != nil {
			return err
		}

		cfg.Proxy.InferenceTokens.Insecure = req.Insecure
		if err := p.saveProvidersConfig(cfg); err != nil {
			return err
		}

		return rpc.ReplyObject(publicInferenceTokens(cfg))
	}

	name := strings.TrimSpace(req.Token.Name)
	if name == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "token name is required")
	}

	hashed, secret, err := p.inferenceAuth.CreateSecret()
	if err != nil {
		return err
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	// FIXME: proxy should just own the config....
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		_ = p.inferenceAuth.RevokeToken(hashed)
		return err
	}

	created := time.Now().UTC()
	cfg.Proxy.InferenceTokens.Tokens = append(cfg.Proxy.InferenceTokens.Tokens, core.InferenceToken{
		Name:    name,
		Token:   hashed,
		Created: created,
	})

	if err := p.saveProvidersConfig(cfg); err != nil {
		_ = p.inferenceAuth.RevokeToken(hashed)
		return err
	}

	p.notifier.Broadcast()

	return rpc.ReplyObject(&inferenceToken{
		Name:      name,
		Token:     Truncate(hashed),
		Secret:    secret,
		CreatedAt: created,
	})
}

func (p *Proxy) inferenceTokenDelete(rpc *jsonrpc.RPC) error {
	id := strings.TrimSpace(rpc.PathVar("id"))
	if id == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "token id is required")
	}

	id = strings.TrimPrefix(id, auth.InferenceTokenPrefix)

	p.lock.Lock()
	defer p.lock.Unlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	i := inferenceTokenIndex(cfg.Proxy.InferenceTokens.Tokens, id)
	if i < 0 {
		return jsonrpc.NewError(http.StatusNotFound, "token not found")
	}

	cfg.Proxy.InferenceTokens.Tokens = append(cfg.Proxy.InferenceTokens.Tokens[:i], cfg.Proxy.InferenceTokens.Tokens[i+1:]...)
	if err := p.saveProvidersConfig(cfg); err != nil {
		return err
	}

	err = p.inferenceAuth.RevokeTokenByKey(id)
	if err != nil {
		return err
	}
	p.notifier.Broadcast()
	return rpc.ReplyObject(map[string]string{"id": id})
}

func (p *Proxy) inferenceTokensList(rpc *jsonrpc.RPC) error {
	p.lock.RLock()
	defer p.lock.RUnlock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}
	return rpc.ReplyObject(publicInferenceTokens(cfg))
}
