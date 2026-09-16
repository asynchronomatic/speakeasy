package mesh

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/negrel/assert"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

const (
	PeerStatusDown    = iota
	PeerStatusUnknown = iota
	PeerStatusRemoved = iota
	PeerStatusUp      = iota
	PeerStatusSync    = iota
)

type PeerEvent struct {
	PeerID string
	Status int
}

type peerStatus struct {
	node   core.PeerNode
	status int
	//ltime       uint64
	needsUpdate bool
}

type DiscoveryManager struct {
	ctrl            *api.MeshClient
	h               host.Host
	node            core.PeerNode
	events          *EventManager
	MDNSEnabled     bool
	onUpdate        core.UpdateHandlerFunc
	discoveryEvents chan PeerEvent

	lock         sync.RWMutex
	ctrlTime     uint64
	registration *api.Registration
	knownPeers   map[string]peerStatus
	allow        *PeerAllowList
}

func (d *DiscoveryManager) loadPeerFromMeshController(id string) (peerStatus, error) {
	var status peerStatus

	peerList, err := d.ctrl.GetPeers()
	if err != nil {
		log.WithName("disc").Warnf("failed to get peer map from controller: %v", err)
		return status, err
	}

	for _, knownPeer := range peerList {
		if knownPeer.ID == id {
			status = peerStatus{
				node:        knownPeer,
				status:      PeerStatusUnknown,
				needsUpdate: true,
			}
			if d.allow != nil {
				d.allow.Add(knownPeer.ID)
			}
			return status, nil
		}
	}
	return status, fmt.Errorf("peer %s not allowed", id)
}

func (d *DiscoveryManager) updatePeersFromAdmin() error {
	valid, ctrlTime, err := d.registration.Refresh()
	if err != nil {
		log.WithName("disc").Errorf("failed to refresh registration: %v", err)
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

	if d.allow != nil {
		ids := make([]string, 0, len(peerList))
		for _, p := range peerList {
			ids = append(ids, p.ID)
		}
		d.allow.ReplaceMembers(ids)
	}

	// update all our known node database to what we just found
	latestNodes := make(map[string]peerStatus)
	for _, peer := range peerList {
		d.lock.Lock()
		knownStatus, ok := d.knownPeers[peer.ID]
		d.lock.Unlock()
		if !ok {
			knownStatus = peerStatus{
				node:        peer,
				status:      PeerStatusUnknown, // will probe
				needsUpdate: true,
			}

			// we are ourselves, and our status never updates
			if peer.ID == d.h.ID().String() {
				knownStatus.status = PeerStatusUp
				knownStatus.needsUpdate = false
			}
		}

		newStatus := knownStatus.status

		if peer.LogicalTime != knownStatus.node.LogicalTime {
			newStatus = PeerStatusDown
		}

		if peer.InstanceId == "" {
			// controller knows nothing about this peer
			newStatus = PeerStatusDown
		}

		log.WithName("disc").Debugf("ctrl reports peer %s (status:%d)", knownStatus.node, knownStatus.status)

		knownStatus = d.updatePeer(knownStatus, newStatus)

		knownStatus.node.LastUpdate = peer.LastUpdate
		knownStatus.node.LogicalTime = peer.LogicalTime
		knownStatus.node.InstanceId = peer.InstanceId

		d.lock.Lock()
		d.knownPeers[peer.ID] = knownStatus
		d.lock.Unlock()
		latestNodes[peer.ID] = knownStatus
	}

	// 1 If a knownPeer is in the list of all nodes we got from the admin server
	// 2. If a node is in our knownNodes, but not in the admin node list.. it was kicked!
	d.lock.Lock()
	for k := range d.knownPeers {
		delete(latestNodes, k)
	}
	d.lock.Unlock()

	// latestNodes should now contain only nodes that are NOT in knownPeers
	for k, v := range latestNodes {
		log.WithName("disc").Warnf("ctrl reports peer %s [HAS BEEN KICKED]", v.node)
		_ = d.updatePeer(v, PeerStatusRemoved)
		d.lock.Lock()
		delete(d.knownPeers, k)
		d.lock.Unlock()
		d.allow.Remove(k)
	}

	d.ctrlTime = ctrlTime
	return nil
}

// TODO: encode this in a staste engine

//	FROM
//	PeerStatusSync           |
//	PeerStatusUnknown        |
//	PeerStatusUnreachable    |
//	PeerStatusDown           |
//	PeerStatusRemoved        |
//
// this is all serialized via the listenForChangesChannel
func (d *DiscoveryManager) updatePeer(currentStatus peerStatus, newStatus int) peerStatus {
	if currentStatus.node.ID == d.h.ID().String() {
		// ignore self in updates
		return currentStatus
	}

	log.WithName("discovery").Debugf("updatePeer(%s) %v N:%v --> %d?", currentStatus.node.ID, currentStatus.status, currentStatus.needsUpdate, newStatus)
	if currentStatus.status == newStatus {
		// statuses do not match
		switch newStatus {
		case PeerStatusSync:
			// sync is transient (one shot)
			_ = d.onUpdate(currentStatus.node, newStatus)
			return currentStatus

		// Keep trying connection
		case PeerStatusUnknown:
			err := d.onUpdate(currentStatus.node, newStatus)
			if err == nil {
				currentStatus.status = PeerStatusUp
				currentStatus.needsUpdate = false
			} else {
				currentStatus.needsUpdate = true // retry
			}

		default:
			return currentStatus
		}
	} else {
		// statuses do not match, taker action
		switch newStatus {
		case PeerStatusSync:
			// sync is transient (one shot)
			_ = d.onUpdate(currentStatus.node, newStatus)
			return currentStatus

		case PeerStatusDown, PeerStatusRemoved:
			_ = d.onUpdate(currentStatus.node, newStatus)
			currentStatus.status = newStatus
			return currentStatus

		case PeerStatusUp, PeerStatusUnknown:
			err := d.onUpdate(currentStatus.node, newStatus)
			if err == nil {
				currentStatus.status = PeerStatusUp
				currentStatus.needsUpdate = false

				// notify peers state changed on us
				_ = d.events.ForceUpdate()
			} else {
				currentStatus.needsUpdate = true // retry
			}
			return currentStatus
		}
	}

	log.WithName("discovery").Debugf("updatePeer(%s) %v N:%v", currentStatus.node.ID, currentStatus.status, currentStatus.needsUpdate)
	return currentStatus
}

// updates can only happen in one thread/goroutine from listenForPeerUpdates
func (d *DiscoveryManager) updatePeerStatus(peerID string, status int) error {
	var err error

	d.lock.Lock()
	knownStatus, ok := d.knownPeers[peerID]
	d.lock.Unlock()
	if !ok {
		knownStatus, err = d.loadPeerFromMeshController(peerID)
		if err != nil {
			log.WithName("disc").Warnf("failed to load peer %s from mesh controller: %v", peerID, err)
			return err
		}
	}

	knownStatus = d.updatePeer(knownStatus, status)
	d.lock.Lock()
	d.knownPeers[peerID] = knownStatus
	d.lock.Unlock()
	return nil
}

func (d *DiscoveryManager) fromHostEvent(e any) (PeerEvent, bool) {
	switch ev := e.(type) {
	case event.EvtPeerConnectednessChanged:
		peerEvent := PeerEvent{
			PeerID: ev.Peer.String(),
		}

		log.WithName("disc").Debugf("%T: %+v", e, ev)
		switch ev.Connectedness {
		case network.Connected:
			peerEvent.Status = PeerStatusUp
		case network.Limited:
			peerEvent.Status = PeerStatusUnknown
		case network.NotConnected:
			peerEvent.Status = PeerStatusUnknown
		default:
			return PeerEvent{}, false
		}
		return peerEvent, true

	default:
		log.WithName("disc").Debugf("%T: %+v", e, ev)

	}
	return PeerEvent{}, false
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
	_ = d.updatePeersFromAdmin()

	dnsSource := NewMDNSEventSource(d.h)
	//go d.listenForMeshEvents(ctx)
	if d.MDNSEnabled {
		go func() {
			time.Sleep(time.Second * 1) // stall to make sure we fail initial proxy bootstrap
			if err := dnsSource.Start(); err != nil {
				log.Panicf("MDNS Failed to start %v\n", err)
			}
			log.WithName("disc").Debugf("MDNS started")
		}()
	}

	hostSub, _ := d.h.EventBus().Subscribe([]any{
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
		case e := <-hostSub.Out():
			evt, ok := d.fromHostEvent(e)
			if !ok {
				continue
			}
			log.WithName("disc").Eventf("P2P HOST Event: Node: %s Status:%d", evt.PeerID, evt.Status)
			_ = d.updatePeerStatus(evt.PeerID, evt.Status)
			continue

		case evt := <-dnsSource.Out():
			log.WithName("disc").Eventf("MDNS Event: Node: %s Status:%d", evt.PeerID, evt.Status)
			_ = d.updatePeerStatus(evt.PeerID, evt.Status)
			continue

		// Rename discovery to system events
		case evt := <-d.discoveryEvents:
			if evt.PeerID == d.node.ID {
				// force a refresh of parents
				log.WithName("disc").Eventf("signaling update for self")
				err := d.events.ForceUpdate()
				if err != nil {
					log.WithName("disc").Warnf("failed to force update via mesh pub: %v", err)
				}

				err = d.registration.SignalUpdate()
				if err != nil {
					log.WithName("disc").Warnf("failed to signal update: %v", err)
				}
				continue
			}

			log.WithName("disc").Eventf("LOCAL Event: Node: %s Status:%d", evt.PeerID, evt.Status)
			_ = d.updatePeerStatus(evt.PeerID, evt.Status)
			continue

		// from mesh topics, we have a bunch of stuff collapsing into the same code paths here including the other
		// topic watcher, should just unify this whole thing
		case evt := <-d.events.Out():
			log.WithName("disc").Eventf("MESH Event: Node: %s Status:%d", evt.PeerID, evt.Status)
			_ = d.updatePeerStatus(evt.PeerID, evt.Status)
			continue

		case <-time.After(time.Minute):
			log.WithName("disc").Eventf("ADMIN EVENT: ")
			_ = d.updatePeersFromAdmin()
			continue

		case <-ctx.Done():
			log.WithName("disc").Eventf("DiscoveryManager stopped")
			return
		}
	}
}

func (d *DiscoveryManager) postEvent(event PeerEvent) {
	d.discoveryEvents <- event
}

// GetPeerMap returns the nodes we know about
func (d *DiscoveryManager) GetPeerMap() (map[string]api.Node, error) {
	var peerMap = make(map[string]api.Node)
	d.lock.RLock()
	defer d.lock.RUnlock()
	for k, v := range d.knownPeers {
		node := v.node
		node.Status = v.status
		peerMap[k] = node

	}
	return peerMap, nil
}

func (d *DiscoveryManager) UpdateHandler(onUpdate core.UpdateHandlerFunc) {
	d.onUpdate = onUpdate
}

func (d *DiscoveryManager) Serve(ctx context.Context) error {
	go d.listenForPeerUpdates(ctx)
	if err := d.events.ForceUpdate(); err != nil {
		log.WithName("evt").Eventf("Failed to force update: %v", err)
	}

	<-ctx.Done()
	return nil
}

func NewDiscoveryManager(a *api.MeshClient, h host.Host, node core.PeerNode, MDNSEnabled bool, allow *PeerAllowList) *DiscoveryManager {
	ev, err := NewEventManager(h)
	if err != nil {
		log.WithName("evt").Eventf("Failed to create event manager: %v", err)
		return nil
	}
	return &DiscoveryManager{
		ctrl:            a,
		h:               h,
		node:            node,
		MDNSEnabled:     MDNSEnabled,
		discoveryEvents: make(chan PeerEvent, 64),
		knownPeers:      make(map[string]peerStatus),
		allow:           allow,
		events:          ev,
	}
}
