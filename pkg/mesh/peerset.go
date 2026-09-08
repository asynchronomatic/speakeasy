package mesh

import (
	"sync"
	"time"
)

const peerAllowRefreshInterval = 2 * time.Second

// PeerAllowList is the proxy-side mesh ACL: relay/self IDs stay pinned,
// and membership is the admin controller's live peer list.
type PeerAllowList struct {
	mu          sync.Mutex
	pinned      map[string]struct{}
	members     map[string]struct{}
	refresher   func() []string
	lastRefresh time.Time
	minInterval time.Duration
}

func NewPeerAllowList() *PeerAllowList {
	return &PeerAllowList{
		pinned:      make(map[string]struct{}),
		members:     make(map[string]struct{}),
		minInterval: peerAllowRefreshInterval,
	}
}

func (a *PeerAllowList) Pin(ids ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range ids {
		if id == "" {
			continue
		}
		a.pinned[id] = struct{}{}
	}
}

func (a *PeerAllowList) SetRefresher(fn func() []string) {
	a.mu.Lock()
	a.refresher = fn
	a.mu.Unlock()
}

func (a *PeerAllowList) ReplaceMembers(ids []string) {
	next := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			next[id] = struct{}{}
		}
	}
	a.mu.Lock()
	a.members = next
	a.mu.Unlock()
}

func (a *PeerAllowList) hasLocked(id string) bool {
	_, ok := a.pinned[id]
	if ok {
		return true
	}
	_, ok = a.members[id]
	return ok
}

func (a *PeerAllowList) Has(id string) bool {
	a.mu.Lock()
	if a.hasLocked(id) {
		a.mu.Unlock()
		return true
	}
	fn := a.refresher
	due := fn != nil && time.Since(a.lastRefresh) >= a.minInterval
	if due {
		a.lastRefresh = time.Now()
	}
	a.mu.Unlock()
	if !due {
		return false
	}
	a.ReplaceMembers(fn())
	a.mu.Lock()
	ok := a.hasLocked(id)
	a.mu.Unlock()
	return ok
}
