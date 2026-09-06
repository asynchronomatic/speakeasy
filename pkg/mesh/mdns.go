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

type discoveryNotifee struct {
	h host.Host
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	log.WithName("mdns").Eventf("MDNS Peer Discovery: %s %+v\n", pi.ID, pi.Addrs)
	// If a peer is broadcast on the local network, connect to it directly!
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// NOTE: we do not need to immediately dial here, we will directly dial via our client
	ctx = network.WithForceDirectDial(ctx, "mdns")
	err := n.h.Connect(ctx, pi)
	if err != nil {
		log.Errorf("MDNS: %v\n", err)
	}
}

// EnableMDNS enables mdns discovery so that nodes running locally to each other can find each other without having
// to report local dns entries
func EnableMDNS(h host.Host) error {
	log.WithName("mdns").Eventf("MDNS Discovery Enabled\n")
	// The second argument is a service tag identifier (keep it matching across your nodes)
	ser := mdns.NewMdnsService(h, "ollama-mesh", &discoveryNotifee{h: h})
	return ser.Start()
}
