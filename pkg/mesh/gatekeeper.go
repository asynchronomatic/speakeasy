package mesh

import (
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

type GateProvider interface {
	Has(s string) bool
}

type GateKeeper struct {
	provider GateProvider
}

// --- relay.ACLFilter ---
func (l *GateKeeper) AllowReserve(p peer.ID, _ ma.Multiaddr) bool {
	allow := l.provider.Has(p.String())
	if !allow {
		log.WithName("gate").Warnf("reserve attempt from %s denied", p)
	}
	return allow
}
func (l *GateKeeper) AllowConnect(src peer.ID, _ ma.Multiaddr, dest peer.ID) bool {
	allow := l.provider.Has(src.String()) && l.provider.Has(dest.String())
	if !allow {
		log.WithName("gate").Warnf("connection attempt from %s to %s denied", src, dest)
	}
	return allow
}

// --- connmgr.ConnectionGater ---

func (l *GateKeeper) InterceptPeerDial(p peer.ID) bool {
	allow := l.provider.Has(p.String())
	if !allow {
		log.WithName("gate").Warnf("dial to %s denied", p)
	}
	return allow
}

// InterceptAddrDial stays open so hole punching can dial predicted addresses
// for an already-allowed peer. Peer identity is checked in InterceptPeerDial.
func (l *GateKeeper) InterceptAddrDial(peer.ID, ma.Multiaddr) bool { return true }

// InterceptAccept stays open: the remote peer ID is not known until Noise.
// Direct/hole-punched inbound is dropped in InterceptSecured if not allow-listed.
func (l *GateKeeper) InterceptAccept(network.ConnMultiaddrs) bool { return true }

func (l *GateKeeper) InterceptSecured(_ network.Direction, p peer.ID, _ network.ConnMultiaddrs) bool {
	allow := l.provider.Has(p.String())
	if !allow {
		log.WithName("gate").Warnf("secured connection from %s denied", p)
	}
	return allow
}
func (l *GateKeeper) InterceptUpgraded(network.Conn) (bool, control.DisconnectReason) {
	return true, 0
}

func NewGateKeeper(p GateProvider) *GateKeeper {
	return &GateKeeper{provider: p}
}
