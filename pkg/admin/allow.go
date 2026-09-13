package admin

import (
	"sync"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

type AllowList struct {
	mu    sync.RWMutex
	allow map[string]struct{}
}

func NewAllowList() (*AllowList, error) {
	l := &AllowList{allow: map[string]struct{}{}}
	return l, nil
}

func (l *AllowList) Has(id string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.allow[id]
	if !ok {
		log.Warnf("acl denied for %s", id)
	}
	return ok
}

func (l *AllowList) Add(id string) {
	l.mu.Lock()
	l.allow[id] = struct{}{}
	l.mu.Unlock()
}

func (l *AllowList) Remove(id string) {
	l.mu.Lock()
	delete(l.allow, id)
	l.mu.Unlock()
}

func (l *AllowList) Peers() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, 0, len(l.allow))
	for id := range l.allow {
		out = append(out, id)
	}
	return out
}
