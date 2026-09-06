package mesh

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

type DiscoveryManager struct {
	admin       *api.MeshClient
	h           host.Host
	node        core.PeerNode
	string      map[peer.ID]struct{}
	MDNSEnabled bool
	onUpdate    core.UpdateHandlerFunc
}

func (d *DiscoveryManager) diffNodes(old, new map[string]api.Node) (map[string]api.Node, map[string]api.Node) {
	selfID := d.h.ID().String()

	addedOrChanged := make(map[string]api.Node)
	removed := make(map[string]api.Node)

	// Find added or changed nodes
	for id, newNode := range new {
		if id == selfID { // filter self
			continue
		}

		if oldNode, exists := old[id]; !exists || oldNode.LogicalTime != newNode.LogicalTime {
			addedOrChanged[id] = newNode
		}
	}

	// Find removed nodes
	for id, oldNode := range old {
		if id == selfID { // filter self
			continue
		}

		if _, exists := new[id]; !exists {
			removed[id] = oldNode
		}
	}

	return addedOrChanged, removed
}

func (d *DiscoveryManager) listenForMeshEvents() {
	sub, _ := d.h.EventBus().Subscribe([]any{
		new(event.EvtLocalAddressesUpdated),
		new(event.EvtAutoRelayAddrsUpdated),
		new(event.EvtLocalReachabilityChanged),
		new(event.EvtHostReachableAddrsChanged),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtPeerConnectednessChanged),
		new(event.EvtPeerIdentificationCompleted),
		new(event.EvtPeerIdentificationFailed),
		new(event.EvtPeerProtocolsUpdated),
		new(event.EvtLocalProtocolsUpdated),
	})
	for e := range sub.Out() {
		switch ev := e.(type) {
		case event.EvtPeerIdentificationCompleted,
			event.EvtPeerConnectednessChanged,
			event.EvtHostReachableAddrsChanged:

			log.WithName("disc").Debugf("%T: %+v", e, ev)
			// Force an update from the system
			_ = d.onUpdate(core.PeerNode{ID: ""}, false)

		default:
			log.WithName("disc").Debugf("%T: %+v", e, ev)
		}
	}
	log.WithName("disc").Fatalf("discovery routine exited")
}

// FIXME: this needs significant rework as we have some cases here that cause
//
//	things to get out of sync
//	- Node Restart ( This SHould cause Logical and LastTimes to increase )
//	- Admin Restart ( This will cause the Logical time to rest , lastTime should increase )
//	- When Admin is reset.. we need to trigger a full reload as there are a bunch of races... like what if logical time catches/passes our last time after the admin resets (etc)
func (d *DiscoveryManager) listenForPeerUpdates() {
	// register self
	var err error
	var registration *api.Registration

	// FIXME: use a retryer
	for {
		registration, err = d.admin.Register(d.node.Name, d.node.ID)
		if err != nil {
			log.WithName("disc").Errorf("failed to register: %v", err)
			time.Sleep(time.Second * 10)
			continue
		}
		break
	}

	lastLogicalTime := uint64(0)
	lastPeers := make(map[string]api.Node)
	for {
		valid, logicalTime, err := registration.Refresh()
		if err != nil {
			time.Sleep(time.Minute)
			continue
		}

		if !valid {
			log.WithName("disc").Warnf("%s lost registration is invalid, reregistering", d.node.ID)
			newReg, err := d.admin.Register(d.node.Name, d.node.ID)
			if err != nil {
				log.WithName("disc").Errorf("failed to register: %v", err)
				time.Sleep(time.Second * 10)
				continue
			}
			registration = newReg
			lastLogicalTime = logicalTime
			continue
		}

		if d.onUpdate == nil {
			time.Sleep(time.Minute)
			continue
		}

		if lastLogicalTime != logicalTime {
			log.WithName("disc").Eventf("mesh.service.discovery: update received [%d | %d]", lastLogicalTime, logicalTime)

			currentPeers, err := d.GetPeerMap()
			if err != nil {
				log.Warnf("mesh.service.discover: failed to get peer map: %v", err)
				time.Sleep(time.Minute)
				continue
			}
			goodUpdate := true

			updatedPeers, removedPeers := d.diffNodes(lastPeers, currentPeers)
			for _, node := range removedPeers {
				log.WithName("disc").Eventf("peer down %s", node.ID)
				if err := d.onUpdate(core.NewPeerNode(node.ID, node.Name), true); err != nil {
					// save the node, removal failed
					n, ok := lastPeers[node.ID]
					if !ok {
						panic("node not found in lastPeers")
					}

					currentPeers[n.ID] = n
					goodUpdate = false
				}
			}

			for _, node := range updatedPeers {
				log.WithName("disc").Eventf("peer updated %s", node.ID)
				if err := d.onUpdate(core.NewPeerNode(node.ID, node.Name), false); err != nil {
					n, ok := currentPeers[node.ID]
					if !ok {
						panic("node not found in currentPeers")
					}
					n.LastUpdate = time.Time{}
					currentPeers[n.ID] = n
					goodUpdate = false
				}
			}

			lastPeers = currentPeers
			if goodUpdate {
				lastLogicalTime = logicalTime
			}
		}
		time.Sleep(time.Minute)
	}
}

// GetPeerMap currently fetches the peers from the admin node
// FIXME: We should keep track of who is actually connected to us instead to reduc the load on the admin server
func (d *DiscoveryManager) GetPeerMap() (map[string]api.Node, error) {
	peers, err := d.admin.GetPeers()
	if err != nil {
		return nil, err
	}
	peerMap := make(map[string]api.Node)
	for _, p := range peers {
		peerMap[p.ID] = p
	}
	return peerMap, nil
}

func (d *DiscoveryManager) UpdateHandler(onUpdate core.UpdateHandlerFunc) {
	d.onUpdate = onUpdate
}

func (d *DiscoveryManager) Serve(ctx context.Context) error {
	go d.listenForMeshEvents()
	go d.listenForPeerUpdates()
	if d.MDNSEnabled {
		go func() {
			time.Sleep(time.Second * 1) // stall to make sure we fail initial proxy bootstrap
			err := EnableMDNS(d.h)
			if err != nil {
				log.Warnf("MDNS Failed to start %v\n", err)
			}
		}()
	}

	<-ctx.Done()
	return nil
}

func NewDiscoveryManager(a *api.MeshClient, h host.Host, node core.PeerNode, MDNSEnabled bool) *DiscoveryManager {
	return &DiscoveryManager{
		admin:       a,
		h:           h,
		node:        node,
		MDNSEnabled: MDNSEnabled,
	}
}
