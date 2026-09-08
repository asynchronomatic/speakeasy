package mesh

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/negrel/assert"
	"golang.org/x/exp/maps"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

const (
	PeerStatusUnknown = iota
	PeerStatusRemoved = iota
	PeerStatusDown    = iota
	PeerStatusUp      = iota
)

type peerEvent struct {
	PeerID string
	Status int
}

type peerStatus struct {
	node        core.PeerNode
	status      int
	ltime       uint64
	needsUpdate bool
}

type DiscoveryManager struct {
	ctrl            *api.MeshClient
	h               host.Host
	node            core.PeerNode
	MDNSEnabled     bool
	onUpdate        core.UpdateHandlerFunc
	discoveryEvents chan peerEvent

	lock         sync.RWMutex
	ctrlTime     uint64
	registration *api.Registration
	knownPeers   map[string]peerStatus
}

// FIXME: exit loop on context close
func (d *DiscoveryManager) listenForMeshEvents(ctx context.Context) {
	defer log.WithName("disc").Eventf("discovery routine exited")

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
	for {
		select {
		case e := <-sub.Out():
			switch ev := e.(type) {
			case event.EvtPeerConnectednessChanged:
				log.WithName("disc").Eventf("%T: %+v", e, ev)
				if ev.Connectedness == network.Connected {
					d.postEvent(peerEvent{
						PeerID: ev.Peer.String(),
						Status: PeerStatusUp,
					})
				} else {
					d.postEvent(peerEvent{
						PeerID: ev.Peer.String(),
						Status: PeerStatusDown,
					})
				}
			case event.EvtPeerIdentificationCompleted:
				log.WithName("disc").Eventf("%T: %+v", e, ev)
				d.postEvent(peerEvent{
					PeerID: ev.Peer.String(),
					Status: PeerStatusUp,
				})

			case event.EvtHostReachableAddrsChanged:
				log.WithName("disc").Debugf("%T: %+v", e, ev)
				// Force an update from the system
				//_ = d.onUpdate(core.PeerNode{ID: ""}, false)

			default:
				log.WithName("disc").Debugf("%T: %+v", e, ev)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *DiscoveryManager) loadPeersFromMeshController() map[string]peerStatus {
	valid, ctrlTime, err := d.registration.Refresh()
	if err != nil {
		log.WithName("disc").Errorf("failed to refers registration: %v", err)
		return nil
	}

	if !valid {
		log.WithName("disc").Warnf("%s lost registration is invalid, reregistering", d.node.ID)
		newReg, err := d.ctrl.Register(d.node.Name, d.node.ID)
		if err != nil {
			log.WithName("disc").Errorf("failed to register: %v", err)
			return nil
		}
		log.WithName("disc").Infof("reregistered node %v", newReg)
		d.registration = newReg
		d.ctrlTime = 0
	}

	if ctrlTime < d.ctrlTime {
		assert.Fail("Ctrl time is not increasing")
	}

	peerList, err := d.ctrl.GetPeers()
	if err != nil {
		log.WithName("disc").Warnf("failed to get peer map from controller: %v", err)
		return nil
	}

	peerUpdates := make(map[string]peerStatus)
	d.lock.Lock()
	defer d.lock.Unlock()

	// update all our known node database to what we just found
	for _, peer := range peerList {
		knownPeer, ok := d.knownPeers[peer.ID]
		if !ok {
			knownPeer = peerStatus{
				node:        peer,
				ltime:       peer.LogicalTime,
				status:      PeerStatusUnknown, // will probe
				needsUpdate: true,
			}
			log.WithName("disc").Eventf("ctrl reports new peer %s", knownPeer.node)
		}

		if knownPeer.ltime != peer.LogicalTime {
			log.WithName("disc").Debugf("ctrl reports ltime change for peer %s  (%v!=%v)", knownPeer.node, knownPeer.ltime, peer.LogicalTime)
			knownPeer.needsUpdate = true
			knownPeer.ltime = peer.LogicalTime
		}

		if knownPeer.needsUpdate {
			peerUpdates[peer.ID] = knownPeer
		}

		log.WithName("disc").Debugf("ctrl reports peer %s NeedsUpdate: %v", knownPeer.node, knownPeer.needsUpdate)
	}

	/* FIXME: remove nodes the controller does not want anymore
	for peerID := range d.knownPeers {
		if _, ok := peerUpdates[peerID]; !ok {
			knownPeer := d.knownPeers[peerID]
			knownPeer.status = PeerStatusRemoved
			peerUpdates[peerID] = knownPeer
		}
	}*/
	d.ctrlTime = ctrlTime
	return peerUpdates
}

func (d *DiscoveryManager) updatePeers(peerUpdates map[string]peerStatus) {
	for k, newState := range peerUpdates {
		// Ignore self forom the per update list
		if newState.node.ID == d.node.ID {
			newState.needsUpdate = false
			newState.status = PeerStatusUp
			peerUpdates[k] = newState
			continue
		}

		bRemove := false
		if newState.status == PeerStatusRemoved || newState.status == PeerStatusDown {
			bRemove = true
		}

		if err := d.onUpdate(newState.node, bRemove); err == nil {
			newState.needsUpdate = false
			if !bRemove {
				newState.status = PeerStatusUp
			}
			peerUpdates[k] = newState
		}
	}

	d.lock.Lock()
	maps.Copy(d.knownPeers, peerUpdates)
	d.lock.Unlock()
}

func (d *DiscoveryManager) listenForPeerUpdates(ctx context.Context) {
	var err error
	defer d.ctrl.Unregister(d.node.ID) // fast remove

	// FIXME: use a retrier
	for {
		d.registration, err = d.ctrl.Register(d.node.Name, d.node.ID)
		if err != nil {
			log.WithName("disc").Errorf("failed to register: %v", err)
			time.Sleep(time.Second * 10)
			continue
		}
		break
	}

	// initial seed from controller
	log.WithName("disc").Debugf("loading peer nodes from controller")
	d.updatePeers(d.loadPeersFromMeshController())

	go d.listenForMeshEvents(ctx)
	if d.MDNSEnabled {
		go func() {
			time.Sleep(time.Second * 1) // stall to make sure we fail initial proxy bootstrap
			err := EnableMDNS(d)
			if err != nil {
				log.Warnf("MDNS Failed to start %v\n", err)
			}
		}()
	}

	for {
		peerUpdates := make(map[string]peerStatus)

		select {
		case evt := <-d.discoveryEvents:
			log.WithName("disc").Eventf("peer event %+v", evt)
			d.lock.Lock()
			knownPeer, ok := d.knownPeers[evt.PeerID]
			d.lock.Unlock()
			if ok {
				knownPeer.needsUpdate = true
				knownPeer.status = evt.Status
				peerUpdates[knownPeer.node.ID] = knownPeer
			} else {
				peerUpdates = d.loadPeersFromMeshController()
			}

		case <-time.After(time.Minute):
			peerUpdates = d.loadPeersFromMeshController()
		case <-ctx.Done():
			log.WithName("disc").Eventf("DiscoveryManager stopped")
			return
		}
		d.updatePeers(peerUpdates)
	}
}

func (d *DiscoveryManager) postEvent(event peerEvent) {
	d.discoveryEvents <- event
}

// GetPeerMap returns the nodes we know about
func (d *DiscoveryManager) GetPeerMap() (map[string]api.Node, error) {
	var peerMap = make(map[string]api.Node)
	d.lock.RLock()
	defer d.lock.RUnlock()
	for k, v := range d.knownPeers {
		peerMap[k] = v.node
	}
	return peerMap, nil
}

func (d *DiscoveryManager) UpdateHandler(onUpdate core.UpdateHandlerFunc) {
	d.onUpdate = onUpdate
}

func (d *DiscoveryManager) Serve(ctx context.Context) error {
	go d.listenForPeerUpdates(ctx)

	<-ctx.Done()
	return nil
}

func NewDiscoveryManager(a *api.MeshClient, h host.Host, node core.PeerNode, MDNSEnabled bool) *DiscoveryManager {
	return &DiscoveryManager{
		ctrl:            a,
		h:               h,
		node:            node,
		MDNSEnabled:     MDNSEnabled,
		discoveryEvents: make(chan peerEvent, 64),
		knownPeers:      make(map[string]peerStatus),
	}
}
