package proxy

import (
	"net/http"
	"strings"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/proxy/auth"
)

type providersListResponse struct {
	Providers []config.Provider `json:"providers"`
}

func providerIndex(providers []config.Provider, id string) int {
	for i := range providers {
		if providers[i].ID == id {
			return i
		}
	}
	return -1
}

func (p *Proxy) validateProvider(prov *config.Provider) error {
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
	if _, err := config.ParseProviderURL(prov.BaseURL, p.allowPrivate); err != nil {
		return jsonrpc.NewError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func providersWithoutTokens(src []config.Provider) []config.Provider {
	out := make([]config.Provider, len(src))
	copy(out, src)
	for i := range out {
		out[i].Token = ""
	}
	return out
}

func providerWithoutToken(prov config.Provider) config.Provider {
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
	resp := providersListResponse{}

	err := p.cm.ReadConfig(func(config *config.Config) error {
		resp.Providers = providersWithoutTokens(config.Providers)
		return nil
	})
	if err != nil {
		return err
	}
	return rpc.ReplyObject(&resp)
}

func (p *Proxy) providerAddHandler(rpc *jsonrpc.RPC) error {
	var prov config.Provider
	if err := rpc.GetObject(&prov); err != nil {
		return err
	}

	log.Warnf("providerAddHandler %s", prov.ID)

	if err := p.validateProvider(&prov); err != nil {
		log.Warnf("providerAddHandler %s %s", prov.ID, err)
		return err
	}

	err := p.cm.UpdateConfig(func(cfg *config.Config) error {
		log.Warnf("check provider id already exists: %s", prov.ID)
		if providerIndex(cfg.Providers, prov.ID) >= 0 {

			return jsonrpc.NewError(http.StatusConflict, "provider id already exists")
		}
		log.Warnf("providerAddHandler %s %s", prov.ID, "OKIEDOKIE")
		cfg.Providers = append(cfg.Providers, prov)
		return nil
	})
	if err != nil {
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
	var prov config.Provider
	if err := rpc.GetObject(&prov); err != nil {
		return err
	}
	prov.ID = id
	if err := p.validateProvider(&prov); err != nil {
		return err
	}

	err := p.cm.UpdateConfig(func(cfg *config.Config) error {
		i := providerIndex(cfg.Providers, id)
		if i < 0 {
			return jsonrpc.NewError(http.StatusNotFound, "provider not found")
		}

		prov.Token = keepProviderToken(prov.Token, cfg.Providers[i].Token)
		cfg.Providers[i] = prov
		return nil
	})
	if err != nil {
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

	err := p.cm.UpdateConfig(func(cfg *config.Config) error {
		i := providerIndex(cfg.Providers, id)
		if i < 0 {
			return jsonrpc.NewError(http.StatusNotFound, "provider not found")
		}
		cfg.Providers = append(cfg.Providers[:i], cfg.Providers[i+1:]...)
		return nil
	})
	if err != nil {
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

func inferenceTokenIndex(tokens []config.InferenceToken, token string) int {
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

func publicInferenceTokens(cfg *config.Config) inferenceTokenList {
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
		var resp inferenceTokenList

		err := p.cm.UpdateConfig(func(cfg *config.Config) error {
			cfg.Proxy.InferenceTokens.Insecure = req.Insecure
			resp = publicInferenceTokens(cfg)
			return nil
		})
		if err != nil {
			return err
		}
		p.inferenceAuth.SetInsecure(req.Insecure)
		return rpc.ReplyObject(&resp)
	}

	name := strings.TrimSpace(req.Token.Name)
	if name == "" {
		return jsonrpc.NewError(http.StatusBadRequest, "token name is required")
	}

	hashed, secret, err := p.inferenceAuth.CreateSecret()
	if err != nil {
		return err
	}

	created := time.Now().UTC()
	err = p.cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.InferenceTokens.Tokens = append(cfg.Proxy.InferenceTokens.Tokens, config.InferenceToken{
			Name:    name,
			Token:   hashed,
			Created: created,
		})
		return nil
	})
	if err != nil {
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

	err := p.cm.UpdateConfig(func(cfg *config.Config) error {
		i := inferenceTokenIndex(cfg.Proxy.InferenceTokens.Tokens, id)
		if i < 0 {
			return jsonrpc.NewError(http.StatusNotFound, "token not found")
		}
		cfg.Proxy.InferenceTokens.Tokens = append(cfg.Proxy.InferenceTokens.Tokens[:i], cfg.Proxy.InferenceTokens.Tokens[i+1:]...)
		return nil
	})
	if err != nil {
		return err
	}

	if err = p.inferenceAuth.RevokeTokenByKey(id); err != nil {
		return err
	}

	p.notifier.Broadcast()
	return rpc.ReplyObject(map[string]string{"id": id})
}

func (p *Proxy) inferenceTokensList(rpc *jsonrpc.RPC) error {
	var resp inferenceTokenList
	p.cm.ReadConfig(func(cfg *config.Config) error {
		resp = publicInferenceTokens(cfg)
		return nil
	})

	return rpc.ReplyObject(&resp)
}
