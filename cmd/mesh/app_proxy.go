package main

import (
	"fmt"
	"strings"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
	"github.com/asynchronomatic/speakeasy/pkg/proxy"
)

func runProxy(config *core.Config) error {
	if config.Proxy.Password == "" {
		return fmt.Errorf("proxy password is required")
	}

	service, err := mesh.NewService(&config.Mesh, nil)
	if err != nil {
		log.Fatalf("Could not initialize mesh err:%v\n", err)
	}
	p, _ := proxy.NewProxy(service, config.Proxy.Listen, config.Providers, config.PrivateBackendsAllowed())
	p.WithAdminToken(config.Proxy.Password)

	attachAdminController(p, config)
	return core.RunInterruptible(p)
}

func adminControllerAddr(config *core.Config) (addr, secret string, ok bool) {
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

func attachAdminController(p *proxy.Proxy, config *core.Config) {
	addr, secret, ok := adminControllerAddr(config)
	if !ok {
		return
	}
	p.WithAdminController(api.NewClient(addr, secret).Admin())
}

func runHybrid(config *core.Config) error {
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
