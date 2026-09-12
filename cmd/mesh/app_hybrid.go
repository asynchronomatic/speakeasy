package main

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/admin"
	"github.com/asynchronomatic/speakeasy/pkg/autoip"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
	"github.com/asynchronomatic/speakeasy/pkg/proxy"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

/*
 *   For standalone to work we need 3 ports forwarded
 *      4001 - relay for other peers
 *      4002 - Admin port
 *
 *  TODO: walk the user though this
 */
func runHybrid() error {
	var err error
	var adminSvc *admin.Server
	var relaySvc *mesh.Relay
	var adminClient *api.AdminClient
	var service core.MeshServiceProvider

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

	// node key is the proxy peer key
	nodeKey, err := mesh.LoadOrCreateKey("node.key")
	if err != nil {
		return err
	}

	// relay key is used by the relay server to connect other peers to us
	relayKey, err := mesh.LoadOrCreateKey("relay.key")
	if err != nil {
		return err
	}

	id, err := peer.IDFromPrivateKey(nodeKey)
	if err != nil {
		return err
	}

	err = cm.UpdateConfig(func(cfg *config.Config) error {
		adminSvc, err = admin.NewServer(fmt.Sprintf(":%d", cfg.Admin.AdminPort), cfg.Admin.Secret)
		if err != nil {
			return fmt.Errorf("could not initialize relay service. err:%v", err)
		}
		adminSvc.WithAdvertiseURL(fmt.Sprintf("http://%s:%d", dc.Public, cfg.Admin.AdminPort))

		relaySvc, err = mesh.NewRelay(relayKey, []string{dc.Public}, mesh.NewGateKeeper(adminSvc.GetAllowList()), cfg.Admin.RelayPort)
		if err != nil {
			return fmt.Errorf("could not initialize relay service err:%v", err)
		}

		adminSvc.WithRelayAddresses(relaySvc.GetAddresses())
		if cfg.Debug {
			log.Default.SetLevel(log.LogAll)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Start and wait for the admin server to come up
	go func() {
		err = adminSvc.Listen()
		if err != nil {
			log.WithName("hybrid").Fatalf("Could not initialize admin server. Err:%v\n", err)
		}
	}()
	err = adminSvc.Wait(context.Background())
	if err != nil {
		return fmt.Errorf("could not initialize admin server err:%v", err)
	}

	//
	err = cm.UpdateConfig(func(cfg *config.Config) error {
		if cfg.Mesh.Secret == "" {
			// This is a new node, so add it to the admin directly
			meshId, meshSecret, err := adminSvc.AddHybridNode(core.PeerNode{
				ID:   id.String(),
				Name: cfg.Mesh.Name,
			})
			if err != nil {
				return fmt.Errorf("could not add hybrid node: %w", err)
			}

			log.Infof("mesh secret: %s, mesh id: %s", meshSecret, meshId)
			cfg.Mesh.Secret = meshSecret
			cfg.Mesh.MeshId = meshId
		}

		addr, secret, ok := adminControllerAddr(cfg)
		if !ok {
			return nil
		}

		adminClient = api.NewClient(addr, secret).Admin()

		service, err = mesh.NewService(&cfg.Mesh, nil, mesh.WithRelayAddrs(
			[]string{fmt.Sprintf("/ip4/%s/udp/%d/quic-v1/p2p/%s", dc.Outbound, cfg.Admin.RelayPort, relaySvc.ID())}))

		if err != nil {
			return fmt.Errorf("could not initialize mesh: %w", err)
		}

		if dc.IsNAT() {
			fmt.Printf("\n")
			fmt.Printf("Warning: We detected that this host (%s) is behind a NAT (%s). This may cause issues with hole punching.\n", dc.Outbound, dc.Public)
			fmt.Printf("  To address this please forward ports %d(udp+tcp), %d(tcp) to %s\n", cfg.Admin.RelayPort, cfg.Admin.AdminPort, dc.Outbound)
			fmt.Printf("\n")
		}
		return nil
	})
	if err != nil {
		return err
	}

	p, _ := proxy.NewProxy(service, cm)

	p.InformNAT = dc.IsNAT()
	p.WithAdminController(adminClient)
	return core.RunInterruptible(relaySvc, p)
}
