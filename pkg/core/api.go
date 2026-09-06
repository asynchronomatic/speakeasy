package core

import (
	"fmt"
	"net/http"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/jsonclient"
)

type PeerConnectionDetails struct {
	PeerID        string
	PeerName      string
	Kind          string
	RemoteAddress string
	LocalAddress  string
	Direction     string
	Security      string
	Multiplexer   string
	StreamCount   int
	Streams       []string
}

type MeshInfo struct {
	AdvertisedAddresses []string
	Connections         []PeerConnectionDetails
}

type PeerNode struct {
	ID          string
	Name        string
	LogicalTime uint64
	LastUpdate  time.Time
}

func (p PeerNode) String() string {
	return fmt.Sprintf("%s/%s", p.ID, p.Name)
}

func NewPeerNode(id, name string) PeerNode {
	return PeerNode{
		ID:   id,
		Name: name,
	}
}

type UpdateHandlerFunc func(peer PeerNode, removed bool) error

type MeshServiceProvider interface {
	Node() PeerNode
	Connect() error
	Disconnect() error
	ClientForPeer(dest PeerNode, longLived bool) jsonclient.Doer // Doer probably belongs in a common package
	ProxyToNode(dest PeerNode, w http.ResponseWriter, r *http.Request)

	GetPeerMeshInfo(node PeerNode) *MeshInfo
	GetPeerMap() (map[string]PeerNode, error)

	WithHandlerFunc(h http.HandlerFunc)
	WithUpdateHandlerFunc(h UpdateHandlerFunc)

	AdminAddress() string
}
