package mesh

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/host/observedaddrs"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

type Relay struct {
	host host.Host
	done chan struct{}
}

func (r *Relay) ID() string {
	return r.host.ID().String()
}

func (r *Relay) GetAddresses() []string {
	addrs := []string{}

	for _, a := range r.host.Addrs() {
		addrs = append(addrs, fmt.Sprintf("%s/p2p/%s", a.String(), r.host.ID().String()))
	}
	return addrs
}

func (r *Relay) Serve(ctx context.Context) error {
	defer r.host.Close()

	rc := relay.DefaultResources()
	/*
		rc.Limit = &relay.RelayLimit{
			Duration: 30 * time.Minute,
			Data:     1 << 30, // 1 GiB each way
		}*/

	log.Debugf("Relay resources: %+v", rc)

	relaySvc, err := relay.New(r.host, relay.WithResources(rc))
	if err != nil {
		log.Panicf("failed to create relay: %v", err)
	}
	defer relaySvc.Close()

	fmt.Println("Secure Public Relay Server Started!")
	fmt.Println("Relay Addresses:")
	for _, addr := range r.host.Addrs() {
		fmt.Printf("    %s/p2p/%s\n", addr.String(), r.host.ID().String())
	}

	<-ctx.Done()
	return nil
}

func (r *Relay) Shutdown() error {
	r.done <- struct{}{}
	return nil
}

func NewRelayOnHost(h host.Host) *Relay {
	r := &Relay{
		host: h,
		done: make(chan struct{}),
	}
	return r
}

func NewRelay(privateKey crypto.PrivKey, publicAddress []string, gate *GateKeeper, relayPort int) (*Relay, error) {
	observedaddrs.ActivationThresh = 1

	options := []libp2p.Option{
		libp2p.Identity(privateKey),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", relayPort),
		),
		libp2p.EnableHolePunching(),
		libp2p.EnableNATService(),
		libp2p.EnableAutoNATv2(),
		libp2p.ForceReachabilityPublic(),
	}

	if publicAddress != nil {
		publicMA := make([]ma.Multiaddr, 0)

		for _, a := range publicAddress {
			publicQuic := ma.StringCast(fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", a, relayPort))
			//privateQuic := ma.StringCast(fmt.Sprintf("/ip4/10.0.0.30/udp/%d/quic-v1", relayPort)) // SEB TEST
			//publicMA = append(publicMA, publicQuic, privateQuic)

			publicMA = append(publicMA, publicQuic)
		}

		options = append(options, libp2p.AddrsFactory(func([]ma.Multiaddr) []ma.Multiaddr {
			return publicMA
		}))
	}

	if gate != nil {
		options = append(options, libp2p.ConnectionGater(gate))
	}

	h, err := libp2p.New(options...)
	if err != nil {
		return nil, err
	}

	log.Infof("NewRelay: ")
	for _, addr := range h.Addrs() {
		log.Infof("    %s/p2p/%s\n", addr.String(), h.ID().String())

	}
	log.Infof("Listening Addresses: %v\n", h.Addrs())
	log.Infof("-- ")

	return NewRelayOnHost(h), nil
}
