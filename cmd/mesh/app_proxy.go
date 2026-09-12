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
	pw, err := promptProxyPassword()
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

func runHybrid() error {
	return fmt.Errorf("hybrid mode not implemented")
}

/*
func runHybrid(config *core.Config) error {
	// load our node key (or create a new one)
	key, err := mesh.LoadOrCreateKey("node.key")
	if err != nil {
		return err
	}

	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}

	discoveredPublicAddress := discoverPublicAddress(config)
	if discoveredPublicAddress == "" {
		log.Errorf("Could not discover a public address. Please set public_address to the ip address of the machine.\n")
		return nil
	}

	config.Mesh.PublicAddress = discoveredPublicAddress
	config.Mesh.AppPort = config.Mesh.RelayPort

	// this is really the ADMIN server
	admin, err := admin.NewServer(fmt.Sprintf(":%d", config.Mesh.AdminPort), config.Mesh.AdminKey)
	if err != nil {
		log.Fatalf("Could not initialize relay service. Err:%v\n", err)
	}

	// Chicken and egg game Hardcode this
	relayAddress := []string{
		fmt.Sprintf("/ip4/%s/udp/%d/quic-v1/p2p/%s", config.Mesh.PublicAddress, config.Mesh.AppPort, id),
		fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", config.Mesh.PublicAddress, config.Mesh.AppPort, id),
		fmt.Sprintf("/ip4/10.0.0.30/udp/%d/quic-v1/p2p/%s", config.Mesh.AppPort, id),
		fmt.Sprintf("/ip4/10.0.0.30/tcp/%d/p2p/%s", config.Mesh.AppPort, id),
	}
	admin.WithRelayAddresses(relayAddress)

	//
	go func() {
		err = admin.Listen()
		if err != nil {
			log.Fatalf("Could not initialize admin server. Err:%v\n", err)
		}
	}()
	// waits for admin server to start
	err = admin.Wait(context.Background())
	if err != nil {
		log.Fatalf("Could not initialize admin server. Err:%v\n", err)
	}

	// When we are running in hybrid mode the admin server is local to us
	config.Mesh.AdminAddress = fmt.Sprintf("http://localhost:%d", config.Mesh.AdminPort)

	service, err := mesh.NewService(&config.Mesh, mesh.NewGateKeeper(admin.GetAllowList()))
	if err != nil {
		log.Fatalf("Could not initialize mesh err:%v\n", err)
	}

	relayService, err := p2prelay.New(service.GetHost())
	if err != nil {
		log.Fatalf("Could not initialize relay service. Err:%v\n", err)
	}
	defer relayService.Close()

	p, _ := proxy.NewProxy(service, config.Proxy.Listen, config.Providers, config.PrivateBackendsAllowed())
	return core.RunInterruptible(p)
}
*/
