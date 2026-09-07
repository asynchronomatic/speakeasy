package mesh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/host/observedaddrs"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/asynchronomatic/speakeasy/pkg/jsonclient"
	"github.com/asynchronomatic/speakeasy/pkg/log"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/autoip"
	"github.com/asynchronomatic/speakeasy/pkg/core"
)

func init() {
	observedaddrs.ActivationThresh = 1
	log.Infof("observed addrs activation threshold set to 1")
}

const (
	streamDialAttempts = 6
	streamDialTimeout  = 30 * time.Second
)

type Service struct {
	node      core.PeerNode
	h         host.Host
	res       *client.Reservation
	relayInfo []peer.AddrInfo
	ctrl      *api.MeshClient // mesh control api
	peers     map[string]peer.ID
	handler   http.HandlerFunc
	config    *core.MeshConfig
	discovery *DiscoveryManager
}

func (m *Service) Close() error {
	return m.h.Close()
}

func (m *Service) connectNode(destNode string) peer.ID {
	if destID, ok := m.peers[destNode]; ok {
		return destID
	}

	// Only record the circuit address here. An eager Connect() that fails
	// puts the peer in swarm dial backoff, so the follow-up NewStream is
	// rejected immediately ("dial backoff") instead of retrying.
	destID := AddDestViaRelay(m.h, m.relayInfo[0], destNode)
	m.peers[destNode] = destID
	return destID
}

func (m *Service) clearDialBackoff(id peer.ID) {
	if sw, ok := m.h.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(id)
	}
}

func isTransientDialErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, swarm.ErrDialBackoff) {
		return true
	}
	s := err.Error()
	for _, sub := range []string{
		"dial backoff",
		"all dials failed",
		"failed to dial",
		"no addresses",
		"NO_RESERVATION",
		"connection refused",
		"connection reset",
		"i/o timeout",
		"timed out",
		"deadline exceeded",
		"transient connection",
		"limited connection",
		"we don't have a connection to peer",
		"stream reset",
	} {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// openStreamDirect attempts to open a stream on a direct connection, if we do not have a direct connection a dial is attempted
func (m *Service) openStreamDirect(ctx context.Context, dest peer.ID, proto protocol.ID) (network.Stream, error) {
	for _, c := range m.h.Network().ConnsToPeer(dest) {
		if !c.Stat().Limited {
			return m.h.NewStream(ctx, dest, proto) // no WithAllowLimitedConn
		}
	}
	lctx := network.WithForceDirectDial(ctx, "direct")
	err := m.h.Connect(lctx, peer.AddrInfo{ID: dest})
	if err != nil {
		log.WithName("mesh").Debugf("direct dial failure to %s%s", dest, proto)
		return nil, err
	}
	log.WithName("mesh").Debugf("direct dial success to %s%s", dest, proto)

	for _, c := range m.h.Network().ConnsToPeer(dest) {
		if !c.Stat().Limited {
			return m.h.NewStream(ctx, dest, proto) // no WithAllowLimitedConn
		}
	}
	return nil, fmt.Errorf("no direct connection to %s available (waiting for hole-punch)", dest)
}

// openStream attempts to establish a direct connection to the provided peer ID using the specified protocols.
// If a direct connection cannot be established, it triggers a circuit relay connection as a fallback.
// Returns the established network stream or an error if no connection can be made.
func (m *Service) openStream(ctx context.Context, dest peer.ID, longLived bool, proto protocol.ID) (network.Stream, error) {
	// for the mesh proxy to work we need a direct connection, circuit (relay/limited) connections are short lived
	// and will result in long queries to ollama to timeout (connections only live for a couple minutes)
	// TODO: we could use the relay for some of our short-lived commands like getting member status and models as those
	//   are short
	if s, err := m.openStreamDirect(ctx, dest, proto); err == nil {
		return s, nil
	}

	// If we can't get a direct connection, initiate connect to the circuit address (which may be limited)
	// this is to force hole punching...
	anyCtx := network.WithAllowLimitedConn(ctx, string(proto))
	if err := m.h.Connect(anyCtx, peer.AddrInfo{ID: dest}); err == nil {
		if !longLived {
			// the client indicated that the stream will be short-lived ... so we can give it a relay/limited stream
			return m.h.NewStream(anyCtx, dest, proto)
		}
	}
	// could not acquire a connection to dest
	return nil, fmt.Errorf("no direct connection to %s available (waiting for hole-punch)", dest)
}

func (m *Service) NewStream(destNode string, longLived bool, proto protocol.ID) (network.Stream, error) {
	destID := m.connectNode(destNode) // ensure node is in our address tables
	return m.openStream(context.Background(), destID, longLived, proto)
}

func (m *Service) ClientForPeer(peer core.PeerNode, longLived bool) jsonclient.Doer {
	destID := m.connectNode(peer.ID)
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			s, err := m.openStream(ctx, destID, longLived, OllamaProtocol)
			if err != nil {
				return nil, err
			}
			return streamConn{s}, nil
		},
		ForceAttemptHTTP2: false,
		// Optional: disable keep-alives if the far side closes after one request
		// DisableKeepAlives: true,
	}
	return &http.Client{Transport: tr, Timeout: 45 * time.Second}
}

func (m *Service) ProxyToNode(destNode core.PeerNode, w http.ResponseWriter, r *http.Request) {
	stream, err := m.NewStream(destNode.ID, true, OllamaProtocol)
	if err != nil {
		log.Printf("could not contact peer: %s err:%v", destNode, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer stream.Close()

	if err := r.Write(stream); err != nil {
		stream.Reset()
		log.Printf("peer write interrupted: %s err:%v", destNode, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	buf := bufio.NewReader(stream)
	resp, err := http.ReadResponse(buf, r)
	if err != nil {
		stream.Reset()
		log.Printf("peer write interrupted: %s err:%v", destNode, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)

	if err != nil {
		log.Printf("peer copy interrupted: %s err:%v", destNode, err)
	}
}

func (m *Service) Node() core.PeerNode {
	return m.node
}

func (m *Service) WithHandlerFunc(h http.HandlerFunc) {
	m.handler = h
}

func (m *Service) AdminAddress() string {
	return m.config.Address
}

func (m *Service) WithUpdateHandlerFunc(h core.UpdateHandlerFunc) {
	m.discovery.UpdateHandler(h)
}

func (m *Service) streamHandler(stream network.Stream) {
	defer stream.Close()

	buf := bufio.NewReader(stream)
	req, err := http.ReadRequest(buf)
	if err != nil {
		stream.Reset()
		log.Warnf("MeshService:streamHandler err:%v", err)
		return
	}
	defer req.Body.Close()

	// Indicate that this is coming from the mesh, this is used to prevent
	// recursing requests back into the mesh
	req.RemoteAddr = stream.Conn().RemotePeer().String()
	req.Header.Set("X-Mesh", "true")
	m.handler.ServeHTTP(&streamResponseWriter{s: stream}, req)
}

func (m *Service) GetPeerConnKind(id string) string {
	peerID, ok := m.peers[id]
	if !ok {
		return ""
	}

	for _, c := range m.h.Network().ConnsToPeer(peerID) {
		fmt.Println(ConnKind(c), c.RemoteMultiaddr())
	}

	fmt.Println("my protocols:")
	for _, p := range m.h.Mux().Protocols() {
		fmt.Println(" ", p)
	}

	return ""
}

// FIXME: thhis should get the info from discovery
func (m *Service) GetPeerMap() (map[string]core.PeerNode, error) {
	peers, err := m.ctrl.GetPeers()
	if err != nil {
		return nil, err
	}
	peerMap := make(map[string]core.PeerNode)
	for _, p := range peers {
		peerMap[p.ID] = core.PeerNode{
			ID:          p.ID,
			Name:        p.Name,
			LastUpdate:  p.LastUpdate,
			LogicalTime: p.LogicalTime,
		}
	}
	return peerMap, nil
}

func (m *Service) diffNodes(old, new map[string]api.Node) (map[string]api.Node, map[string]api.Node) {
	addedOrChanged := make(map[string]api.Node)
	removed := make(map[string]api.Node)

	// Find added or changed nodes
	for id, newNode := range new {
		if id == m.node.ID { // filter self
			continue
		}

		if oldNode, exists := old[id]; !exists || !oldNode.LastUpdate.Equal(newNode.LastUpdate) {
			addedOrChanged[id] = newNode
		}
	}

	// Find removed nodes
	for id, oldNode := range old {
		if id == m.node.ID { // filter self
			continue
		}

		if _, exists := new[id]; !exists {
			removed[id] = oldNode
		}
	}

	return addedOrChanged, removed
}

func (m *Service) GetPeerMeshInfo(node core.PeerNode) *core.MeshInfo {
	info := core.MeshInfo{
		AdvertisedAddresses: make([]string, 0),
		Connections:         nil,
	}

	targetPeerID, err := peer.Decode(node.ID)
	if err != nil {
		return &info
	}

	advertisedAddrs := m.h.Peerstore().Addrs(targetPeerID)
	for _, a := range advertisedAddrs {
		info.AdvertisedAddresses = append(info.AdvertisedAddresses, a.String())
	}

	conns := m.h.Network().Conns()
	for _, conn := range conns {
		cd := core.PeerConnectionDetails{
			PeerName:      "",
			PeerID:        conn.RemotePeer().String(),
			RemoteAddress: conn.RemoteMultiaddr().String(),
			LocalAddress:  conn.LocalMultiaddr().String(),
			Direction:     conn.Stat().Direction.String(),
			Security:      fmt.Sprintf("%s", conn.ConnState().Security),
			Multiplexer:   conn.ConnState().Transport,
			Kind:          ConnKind(conn),
		}
		streams := conn.GetStreams()
		cd.StreamCount = len(streams)
		for _, stream := range streams {
			cd.Streams = append(cd.Streams, fmt.Sprintf("%s", stream.Protocol()))
		}
		info.Connections = append(info.Connections, cd)
	}
	return &info
}

func (m *Service) GetHost() host.Host {
	return m.h
}

// Connect this service to the mesh
func (m *Service) Connect() error {
	log.WithName("mesh").Infof("My PeerNode: %s\n", m.node)

	// FIXME: if the node visibility is forced public it will never see a circuit address
	WaitForAddress(m.h, false)

	// start the discovery subsystem
	go func() {
		m.discovery.Serve(context.Background())
	}()
	return nil
}

func (m *Service) Disconnect() error {
	return nil
}

func NewService(mc *core.MeshConfig, gater connmgr.ConnectionGater) (*Service, error) {
	mesh, err := api.NewClient(mc.Address, mc.Secret).Mesh("default")
	if err != nil {
		return nil, fmt.Errorf("could open mesh admin client. err:%v\n", err)
	}

	// load our node key (or create a new one)
	key, err := LoadOrCreateKey("node.key")
	if err != nil {
		return nil, err
	}

	nodeID, err := NodeIDFromKey(key)
	if err != nil {
		return nil, err
	}

	err = mesh.Login(nodeID, mc.Secret)
	if err != nil {
		return nil, fmt.Errorf("could not login to mesh. err:%v\n", err)
	}

	// Retrieve the bootstrap address of our public relays
	btAddress, err := mesh.GetAddress()
	if err != nil {
		return nil, err
	}

	log.WithName("svc").Debugf("Bootstrap Addresses: %+v\n", btAddress)

	relayInfo := PeerAddrInfoFromMulti(btAddress)

	// FIXME: for limited deploys, we only ask for one public relay
	observedaddrs.ActivationThresh = 1
	opts := []libp2p.Option{
		libp2p.Identity(key),
		libp2p.ListenAddrStrings(
			// Note: App side does not need a specific port ( this can be set in config to 0 )
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", mc.Port),
			//fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", mc.AppPort),
			"/p2p-circuit",
		),
		libp2p.EnableRelay(),
		libp2p.EnableNATService(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableAutoRelayWithStaticRelays(relayInfo,
			autorelay.WithNumRelays(1),
			autorelay.WithMinCandidates(1),
			autorelay.WithNumRelays(1)),
		libp2p.EnableHolePunching(),
	}

	if gater != nil {
		opts = append(opts, libp2p.ConnectionGater(gater))
	}

	isPrivate := true
	switch mc.PublicAddress {
	case "auto":
		if dc, err := autoip.GetPublicAddress(); err == nil {
			if dc.IsPublic() {
				mc.PublicAddress = dc.Public
				isPrivate = false
			}
		}
	default:
		isPrivate = false
	}
	if mc.ForcePrivate {
		isPrivate = true
	}

	if isPrivate {
		log.Eventf("Node Visibility: private")
		opts = append(opts, libp2p.ForceReachabilityPrivate())
	} else {
		log.Eventf("Node Visibility: public Address: %s", mc.PublicAddress)
		opts = append(opts, libp2p.ForceReachabilityPublic())

		// Advertise the public endpoint as well
		pubTCP := ma.StringCast(fmt.Sprintf("/ip4/%s/tcp/4001", mc.PublicAddress))
		pubUDP := ma.StringCast(fmt.Sprintf("/ip4/%s/udp/4001/quic-v1", mc.PublicAddress))
		opts = append(opts, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			return append(addrs, pubTCP, pubUDP)
		}))
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	// Auto authorize the host ( TODO: see if we can do this earlier, because the id is in the key )
	err = mesh.Login(host.ID().String(), mc.Secret)
	if err != nil {
		_ = host.Close()
		return nil, fmt.Errorf("error authorizing host: %v", err)
	}

	// Connect to relay
	for _, r := range relayInfo {
		err = host.Connect(context.Background(), r)
		if err != nil {
			log.Warnf("connection %s error: %v\n", r.ID, err)
		}
	}

	node := core.PeerNode{
		ID:   host.ID().String(),
		Name: mc.Name,
	}

	m := &Service{
		node:      node,
		h:         host,
		ctrl:      mesh,
		relayInfo: relayInfo,
		peers:     make(map[string]peer.ID),
		handler: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not implemented", http.StatusNotImplemented)
		},
		config:    mc,
		discovery: NewDiscoveryManager(mesh, host, node, mc.MDNSEnabled),
	}

	host.SetStreamHandler(OllamaProtocol, m.streamHandler)
	return m, nil
}
