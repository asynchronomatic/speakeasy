package main

import (
	"fmt"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/log"

	"github.com/asynchronomatic/speakeasy/pkg/admin"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
)

func runAdminAndRelay() error {
	// Load (or create a new) the node identity, identity will persist in this file
	key, err := mesh.LoadOrCreateKey("relay.key")
	if err != nil {
		return err
	}

	cm := config.NewManager(config.DefaultConfigPath)
	if err := cm.EnsureLoaded(); err != nil {
		return err
	}

	var adminSvc *admin.Server
	var relaySvc *mesh.Relay

	err = cm.ReadConfig(func(config *config.Config) error {
		discoveredPublicAddress := discoverPublicAddress(config)
		if discoveredPublicAddress == "" {
			log.Errorf("Could  discover a public address. Please set public_address to the ip address of the machine.\n")
			log.Errorf("  - A public address is required to run an relay node\n")
			return nil
		}

		// Init a new admin server... the admin server controls who gets access to our mesh, only nodes with the AdminKey can gain access
		adminSvc, err = admin.NewServer(fmt.Sprintf(":%d", config.Admin.AdminPort), config.Admin.Secret)
		if err != nil {
			return fmt.Errorf("could not initialize admin server err: %v", err)
		}
		adminSvc.WithAdvertiseURL(config.Admin.Address)

		// Initialize a Relay node, this node does not service any application traffic
		//  we gate access using the admin service AllowList as the gatekeeper
		relaySvc, err = mesh.NewRelay(key, []string{discoveredPublicAddress}, mesh.NewGateKeeper(adminSvc.GetAllowList()), config.Admin.RelayPort)
		if err != nil {
			return fmt.Errorf("could not initialize relay service err:%v", err)
		}

		// Admin service needs to expose the relay addresses for incoming clients.
		//  These addresses are how our peer nodes bootstrap teh p2p network
		adminSvc.WithRelayAddresses(relaySvc.GetAddresses())
		return nil
	})
	if err != nil {
		return err
	}

	// Run all of our services
	return core.RunInterruptible(adminSvc, relaySvc)
}
