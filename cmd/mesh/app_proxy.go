package main

import (
	"fmt"
	"strings"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
	"github.com/asynchronomatic/speakeasy/pkg/proxy"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func proxyStart() error {
	var service core.MeshServiceProvider

	cm := config.NewManager(config.DefaultConfigPath)
	if err := cm.EnsureLoaded(); err != nil {
		return err
	}

	// HACK:
	var admin *api.AdminClient

	err := cm.ReadConfig(func(cfg *config.Config) error {
		var err error
		if cfg.Proxy.Password == "" {
			return fmt.Errorf("proxy password is required")
		}

		if service, err = mesh.NewService(&cfg.Mesh, nil); err != nil {
			return fmt.Errorf("could not initialize mesh: %w", err)
		}

		addr, secret, ok := adminControllerAddr(cfg)
		if !ok {
			return nil
		}

		admin = api.NewClient(addr, secret).Admin()
		return nil
	})
	if err != nil {
		return err
	}

	p, _ := proxy.NewProxy(service, cm)
	p.WithAdminController(admin)

	return core.RunInterruptible(p)
}

func proxyConfigSetPassword() error {
	pw, err := askPassword("Protects the local proxy UI and on this node.")
	if err != nil {
		return err
	}

	cm := config.NewManager(config.DefaultConfigPath)
	return cm.UpdateConfig(func(cfg *config.Config) error {
		cfg.Proxy.Password = security.MustPasswordHashAndEncodeBase62(pw)
		return nil
	})
}

func adminControllerAddr(config *config.Config) (addr, secret string, ok bool) {
	secret = strings.TrimSpace(config.Admin.Secret)
	if secret == "" {
		return "", "", false
	}
	addr = strings.TrimSpace(config.Admin.Address)
	if addr == "" {
		addr = strings.TrimSpace(config.Mesh.Address)
	}
	if addr == "" {
		return "", "", false
	}
	return addr, secret, true
}
