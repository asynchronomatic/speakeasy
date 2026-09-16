package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

var ErrDuplicateNode = errors.New("duplicate node")

type meshNodeRecord struct {
	NodeID       string
	Name         string
	AddedAt      time.Time
	InvitedAs    string
	PasswordHash string
}

type NodeReference struct {
	Node core.PeerNode
	rec  meshNodeRecord
}

type MeshNodeStore struct {
	mesh        string
	lock        sync.RWMutex
	nodes       map[string]*NodeReference
	kv          *jsonkv.Store
	logicalTime uint64
	lastUpdate  time.Time
	acl         *AllowList
}

func hashLoginSecret(secret string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func (store *MeshNodeStore) meshNodeKVKey(nodeID string) string {
	return "/mesh/" + store.mesh + "/nodes/" + nodeID
}

func (store *MeshNodeStore) get(nodeId string) (*meshNodeRecord, error) {
	nodeKey := store.meshNodeKVKey(nodeId)
	fmt.Printf("get node: %s\n", nodeKey)

	var rec meshNodeRecord
	if err := store.kv.Get(nodeKey, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// Get retrieves a PeerNode by its nodeId from the MeshNodeStore. Returns the node and true if found, or nil and false otherwise.
func (store *MeshNodeStore) Get(nodeId string) (*core.PeerNode, bool) {
	store.lock.Lock()
	defer store.lock.Unlock()

	if ref, ok := store.nodes[nodeId]; ok {
		node := ref.Node
		return &node, true
	}
	return nil, false
}

// NodeLogin validates a node's credentials by comparing the supplied secret with the stored hashed password. Returns an error if invalid.
func (store *MeshNodeStore) NodeLogin(nodeId, secret string) error {
	rec, err := store.get(nodeId)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyLoginHash, []byte(secret))
		return jsonrpc.NewError(http.StatusUnauthorized, "invalid credentials")
	}
	mismatch := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(secret)) != nil
	if mismatch {
		return jsonrpc.NewError(http.StatusUnauthorized, "invalid credentials")
	}
	return nil
}

// Register assigns a new instance ID and updates logical time for an existing node. Returns an error if the node is invalid.
func (store *MeshNodeStore) Register(nodeId string) (string, uint64, error) {
	instanceId := uuid.New().String()
	store.lock.Lock()
	defer store.lock.Unlock()
	store.logicalTime++
	store.lastUpdate = time.Now().UTC()

	if ref, ok := store.nodes[nodeId]; ok {
		ref.Node.InstanceId = instanceId
		ref.Node.LogicalTime = store.logicalTime
		ref.Node.LastUpdate = time.Now().UTC()
		return instanceId, ref.Node.LogicalTime, nil
	}

	return "", 0, fmt.Errorf("invalid node")
}

// RefreshNode updates the node's last update timestamp and logical time if valid; invalidates if instance ID mismatches.
func (store *MeshNodeStore) RefreshNode(nodeId string, instanceID string, invalidate bool) (bool, error) {
	store.lock.Lock()
	defer store.lock.Unlock()
	if ref, ok := store.nodes[nodeId]; ok {
		ref.Node.LastUpdate = time.Now().UTC()

		if ref.Node.InstanceId != instanceID {
			return false, nil
		}
		if invalidate {
			store.logicalTime++
			ref.Node.LogicalTime = store.logicalTime
		}
		return true, nil
	}
	return false, fmt.Errorf("invalid node")
}

// Unregister removes a node from the MeshNodeStore by resetting its instance ID and logical time. Returns an error if the node is invalid.
func (store *MeshNodeStore) Unregister(nodeId string) error {
	store.lock.Lock()
	defer store.lock.Unlock()
	store.logicalTime++
	store.lastUpdate = time.Now().UTC()
	if ref, ok := store.nodes[nodeId]; ok {
		ref.Node.InstanceId = ""
		ref.Node.LogicalTime = 0
		ref.Node.LastUpdate = time.Now().UTC()
		return nil
	}
	return fmt.Errorf("invalid node")
}

// AddNode adds a new node to the MeshNodeStore, initializing its state and storing its login credentials.
func (store *MeshNodeStore) AddNode(node core.PeerNode, from, secret string) error {
	nodeKey := store.meshNodeKVKey(node.ID)
	fmt.Printf("Adding node: %s\n", nodeKey)

	hash, err := hashLoginSecret(secret)
	if err != nil {
		return err
	}

	_, err = store.get(node.ID)
	if err == nil {
		return ErrDuplicateNode
	}
	if !errors.Is(err, jsonkv.ErrNotFound) {
		return err
	}

	rec := meshNodeRecord{
		NodeID:       node.ID,
		Name:         node.Name,
		AddedAt:      time.Now().UTC(),
		InvitedAs:    from,
		PasswordHash: hash,
	}

	if err := store.kv.Put(nodeKey, &rec); err != nil {
		return err
	}

	ref := &NodeReference{
		Node: core.PeerNode{
			ID:          rec.NodeID,
			Name:        rec.Name,
			LastUpdate:  time.Time{},
			LogicalTime: 0,
		},
		rec: rec,
	}

	store.lock.Lock()
	store.logicalTime++
	store.lastUpdate = time.Now().UTC()
	store.nodes[node.ID] = ref
	store.lock.Unlock()

	store.acl.Add(rec.NodeID)
	return nil
}

// DeleteNode removes a node from the MeshNodeStore and deletes its data from the underlying key-value store.
func (store *MeshNodeStore) DeleteNode(nodeID string) error {
	nodeKey := store.meshNodeKVKey(nodeID)
	fmt.Printf("delete node: %s\n", nodeKey)

	if err := store.kv.Delete(nodeKey); err != nil {
		return err
	}

	store.acl.Remove(nodeID)
	store.lock.Lock()
	if _, ok := store.nodes[nodeID]; ok {
		store.logicalTime++
		store.lastUpdate = time.Now().UTC()
	}
	delete(store.nodes, nodeID)
	store.lock.Unlock()

	return nil

}

// LogicalTime retrieves the current logical time of the MeshNodeStore in a thread-safe manner.
func (store *MeshNodeStore) LogicalTime() uint64 {
	store.lock.RLock()
	defer store.lock.RUnlock()
	return store.logicalTime
}

func (store *MeshNodeStore) NodeList() []api.Node {
	store.lock.RLock()
	defer store.lock.RUnlock()

	apiNodes := make([]api.Node, 0, len(store.nodes))
	for _, ref := range store.nodes {
		apiNodes = append(apiNodes, ref.Node)
	}
	return apiNodes
}

func (store *MeshNodeStore) AdminNodeList() []api.AdminNode {
	store.lock.RLock()
	defer store.lock.RUnlock()

	adminNodes := make([]api.AdminNode, 0, len(store.nodes))
	for _, ref := range store.nodes {

		adminNodes = append(adminNodes, api.AdminNode{
			ID:        ref.rec.NodeID,
			Name:      ref.rec.Name,
			MeshId:    store.mesh,
			AddedAt:   ref.rec.AddedAt,
			InvitedAs: ref.rec.InvitedAs,
		})

	}
	return adminNodes
}

func (store *MeshNodeStore) List(consume func(node *NodeReference)) {
	store.lock.RLock()
	defer store.lock.RUnlock()
	for _, ref := range store.nodes {
		consume(ref)
	}
}

func (store *MeshNodeStore) Load() error {
	prefix := fmt.Sprintf("/mesh/%s/nodes/", store.mesh)

	err := store.kv.ForEach(prefix, func(key string, data []byte) error {
		fmt.Printf("forEach node: %s\n", key)
		nodeID := strings.TrimPrefix(key, prefix)

		var rec meshNodeRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		id := rec.NodeID
		if id == "" {
			id = nodeID
		}

		node := &NodeReference{
			Node: core.PeerNode{
				ID:          id,
				Name:        rec.Name,
				LastUpdate:  time.Time{},
				LogicalTime: 0,
			},
			rec: rec,
		}
		store.nodes[nodeID] = node
		store.acl.Add(nodeID)
		return nil
	})
	return err
}

func (store *MeshNodeStore) ExpireStaleNodes(nodeExpiry time.Duration) {
	store.lock.Lock()
	defer store.lock.Unlock()

	for _, ref := range store.nodes {
		if ref.Node.InstanceId == "" {
			// skip expired nodes
			continue
		}

		if time.Since(ref.Node.LastUpdate) > nodeExpiry {
			log.WithName("nodes").Eventf("expiring stale node: %s\n", ref.Node)
			ref.Node.InstanceId = ""
			ref.Node.LogicalTime = 0
		}
	}
}

func NewMeshNodeStore(mesh string, kv *jsonkv.Store, acl *AllowList) (*MeshNodeStore, error) {
	m := &MeshNodeStore{
		acl:         acl,
		mesh:        mesh,
		kv:          kv,
		nodes:       make(map[string]*NodeReference),
		logicalTime: 0,
		lastUpdate:  time.Now().UTC(),
	}

	err := m.Load()
	if err != nil {
		return nil, err
	}
	return m, nil
}
