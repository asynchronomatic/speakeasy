package main

import (
	"fmt"

	"github.com/asynchronomatic/speakeasy/pkg/autoip"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

func initConfig() error {
	dc, err := autoip.GetPublicAddress()
	if err != nil {
		fmt.Printf("Error: We could not discover your public internet address.")
		fmt.Printf("  A public internet ip address is required to run a mesh node in hybrid mode.")
		return err
	}

	cm := config.NewManager(config.DefaultConfigPath)
	if err = cm.EnsureLoaded(); err != nil {
		// config did not exist, initialize a new one
		err = cm.InitializeFromDefaults()
		if err != nil {
			return fmt.Errorf("initialize config: %w", err)
		}

		// Initializing configs
		pw, err := askPassword("Password for UI and Administration")
		if err != nil {
			return fmt.Errorf("ask password: %w", err)
		}

		if pw == "" {
			panic(pw)
		}

		err = cm.UpdateConfig(func(cfg *config.Config) error {
			cfg.Admin.Address = fmt.Sprintf("http://%s:%d", dc.Public, config.DefaultAdminPort)
			cfg.Admin.Secret = pw // FIXME: hash this too
			cfg.Proxy.Password = security.MustPasswordHashAndEncodeBase62(pw)
			cfg.Mesh.MDNSEnabled = true
			cfg.Mesh.Address = cfg.Admin.Address
			cfg.Mesh.ForcePrivate = true
			cfg.Mesh.Port = 4003
			return nil
		})
		if err != nil {
			return err
		}
	}

	fmt.Printf("Config initialized successfully into: %s\n", config.DefaultConfigPath)
	fmt.Printf("  Documentation: https://github.com/asynchronomatic/speakeasy/blob/main/docs/SETTINGS.md\n")
	return nil
}
