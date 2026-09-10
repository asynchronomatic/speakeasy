# Current Known Issues

## If the admin server restarts we loose information about nodes on the network
Add code to the nodes to occasionally re-reserve with the admin server

## Stoping a node leaves their model routing information stale (add code to prune dead node code)
Detect peer loss and remove their models from the model routing table




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

	hashed, secret, err := p.inferenceAuth.CreateSecret()
	if err != nil {
		return err
	}
	
	p.lock.Lock()
	cfg, err := p.loadProvidersConfig()
	if err != nil {
		return err
	}


	cfg, err := p.loadProvidersConfig()
	

	p.notifier.Broadcast()

	return rpc.ReplyObject(&inferenceToken{
		Name:      req.Token.Name,
		Token:     Truncate(hashed, 15) + "...",
		Secret:    secret,
		CreatedAt: time.Now(),
	})
}
