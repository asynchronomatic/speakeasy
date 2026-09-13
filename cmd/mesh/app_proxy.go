package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
	"github.com/asynchronomatic/speakeasy/pkg/proxy"
	"github.com/asynchronomatic/speakeasy/pkg/secrets"
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

		admin, _ = adminControllerFromEnv(cfg.Admin.Address)
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
		cfg.Proxy.Password = secrets.MustPasswordHashAndEncodeBase62(pw)
		return nil
	})
}

func adminControllerFromEnv(address string) (client *api.AdminClient, ok bool) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, false
	}

	secret := os.Getenv("SPEAKEASY_ADMIN_SERVER_SECRET")
	if secret == "" {
		return nil, false
	}

	admin := api.NewClient(address, secret).Admin()
	_, err := admin.ListNodes()
	if err != nil {
		return nil, false
	}

	return admin, true
}
