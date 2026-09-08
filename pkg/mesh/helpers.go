package mesh

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

// FIXME: move this to proxy
const OllamaProtocol = "/ollama/0.0.1"

func CircuitAddr(relayInfo peer.AddrInfo, dest peer.ID) (ma.Multiaddr, error) {
	// FIXME: we really should not be generating a circuit address ourselves, it should be passsed to us by a peer
	return ma.NewMultiaddr(
		relayInfo.Addrs[0].String() + "/p2p/" + relayInfo.ID.String() +
			"/p2p-circuit/p2p/" + dest.String(),
	)
}

func ParseAddrInfo(s string) (peer.AddrInfo, error) {
	ai, err := peer.AddrInfoFromString(s)
	if err == nil {
		return *ai, nil
	}
	// Allow a bare peer ID; caller must supply circuit addrs separately.
	id, idErr := peer.Decode(s)
	if idErr != nil {
		return peer.AddrInfo{}, fmt.Errorf("parse %q: %v (also not a peer id: %v)", s, err, idErr)
	}
	return peer.AddrInfo{ID: id}, nil
}

func AddDestViaRelay(h host.Host, relayInfo peer.AddrInfo, dest string) peer.ID {
	destInfo, err := ParseAddrInfo(dest)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// If the user already passed a /p2p-circuit address, just store it.
	hasCircuit := false
	for _, a := range destInfo.Addrs {
		if _, err := a.ValueForProtocol(ma.P_CIRCUIT); err == nil {
			hasCircuit = true
			break
		}
	}
	if hasCircuit {
		h.Peerstore().AddAddrs(destInfo.ID, destInfo.Addrs, peerstore.PermanentAddrTTL)
		return destInfo.ID
	}

	circ, err := CircuitAddr(relayInfo, destInfo.ID)
	if err != nil {
		log.Fatalf("%v", err)
	}
	log.Printf("CIRC %s: %+v", dest, circ)
	h.Peerstore().AddAddr(destInfo.ID, circ, peerstore.PermanentAddrTTL)
	return destInfo.ID
}

func PrintAddrs(prefix string, h host.Host) {
	fmt.Println(prefix)
	for _, a := range h.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, h.ID())
	}
}

func LoadOrCreateKey(path string) (crypto.PrivKey, error) {
	if path == "" {
		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		return priv, err
	}
	if b, err := os.ReadFile(path); err == nil {
		return crypto.UnmarshalPrivateKey(b)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	b, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return priv, os.WriteFile(path, b, 0o600)
}

// NodeIDFromKey returns the libp2p peer ID string for a key from LoadOrCreateKey.
func NodeIDFromKey(key crypto.PrivKey) (string, error) {
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func ConnKind(c network.Conn) string {
	if _, err := c.RemoteMultiaddr().ValueForProtocol(ma.P_CIRCUIT); err == nil {
		if c.Stat().Limited {
			return "relay/limited"
		}
		return "relay/unlimited"
	}
	if c.Stat().Limited {
		return "limited"
	}
	return "direct"
}

func WaitForAddress(h host.Host, once bool) string {
	for {
		log.WithName("mesh").Debugf("Waiting for circuit addresses\n")
		hasCircuit := ""
		for _, a := range h.Addrs() {
			if strings.HasSuffix(a.String(), "p2p-circuit") {
				hasCircuit = a.String()
			}
		}

		if hasCircuit != "" {
			log.WithName("mesh").Debugf("My Addresses:\n")
			for _, a := range h.Addrs() {
				log.Debugf("  %s\n", a)
			}
			return hasCircuit
		}

		if once {
			return ""
		}
		time.Sleep(5 * time.Second)
	}
}

func PeerAddrInfoFromMulti(addrList []string) []peer.AddrInfo {
	var relays []peer.AddrInfo
	byID := map[peer.ID]*peer.AddrInfo{}

	for _, s := range addrList {
		ai, err := peer.AddrInfoFromString(s)
		if err != nil {
			panic(err)
		}
		if existing, ok := byID[ai.ID]; ok {
			existing.Addrs = append(existing.Addrs, ai.Addrs...)
			continue
		}
		cp := *ai
		byID[ai.ID] = &cp
	}

	for _, ai := range byID {
		relays = append(relays, *ai)
	}

	return relays
}
