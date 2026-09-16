package mesh

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

// If your nodes are on the same local network, they shouldn't have to use the public relay to discover each other
// in the first place. You can deploy mDNS (Multicast DNS) discovery.mDNS broadcasts a signal over your local router
// subnet so nodes can find each other instantly, establishing a direct connection without ever querying the remote
// relay for information.
/*
type discoveryNotifee struct {
	discovery *DiscoveryManager
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	log.WithName("mdns").Eventf("MDNS Peer Discovery: %s %+v\n", pi.ID, pi.Addrs)
	// If a peer is broadcast on the local network, connect to it directly!
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// NOTE: we do not need to immediately dial here, we will directly dial via our client
	ctx = network.WithForceDirectDial(ctx, "mdns")
	err := n.discovery.h.Connect(ctx, pi)
	if err != nil {
		log.Errorf("MDNS: %v\n", err)
	}

	n.discovery.postEvent(PeerEvent{
		PeerID: pi.ID.String(),
		Status: PeerStatusUp,
	})
}

// EnableMDNS enables mdns discovery so that nodes running locally to each other can find each other without having
// to report local dns entries
func EnableMDNS(d *DiscoveryManager) error {
	log.WithName("mdns").Eventf("MDNS Discovery Enabled\n")
	// The second argument is a service tag identifier (keep it matching across your nodes)
	ser := mdns.NewMdnsService(d.h, "ollama-mesh", &discoveryNotifee{discovery: d})
	return ser.Start()
}
*/

type MDNSEventSource struct {
	h    host.Host
	c    chan PeerEvent
	mdns mdns.Service
}

func (s *MDNSEventSource) HandlePeerFound(pi peer.AddrInfo) {
	log.WithName("MDNS").Eventf("LAN Peer found: %s", pi.ID.String())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// NOTE: force a direct dial here, to get a direct connection on lan
	ctx = network.WithForceDirectDial(ctx, "mdns")
	err := s.h.Connect(ctx, pi)
	if err != nil {
		log.Errorf("MDNS: %v\n", err)
	}

	select {
	case s.c <- PeerEvent{
		PeerID: pi.ID.String(),
		Status: PeerStatusConnecting,
	}:
	}
}

func (s *MDNSEventSource) Start() error {
	return s.mdns.Start()
}

func (s *MDNSEventSource) Out() <-chan PeerEvent {
	return s.c
}

func NewMDNSEventSource(h host.Host) *MDNSEventSource {
	s := &MDNSEventSource{
		h: h,
		c: make(chan PeerEvent, 64),
	}

	s.mdns = mdns.NewMdnsService(s.h, "speakeasy", s)
	return s
}
