package admin

import (
	"fmt"
	"sync"
	"testing"
)

func TestNewAllowListEmptyPath(t *testing.T) {
	l, err := NewAllowList()
	if err != nil {
		t.Fatal(err)
	}
	if l.Has("anyone") {
		t.Fatal("empty allow list should deny")
	}
	if peers := l.Peers(); len(peers) != 0 {
		t.Fatalf("expected no peers, got %v", peers)
	}

	l.Add("peer-1")
	if !l.Has("peer-1") {
		t.Fatal("Add with empty path should still allow in memory")
	}
}

func TestAllowListAddRemoveMemory(t *testing.T) {
	l, err := NewAllowList()
	if err != nil {
		t.Fatal(err)
	}

	l.Add("peer-1")
	if !l.Has("peer-1") {
		t.Fatal("Add should allow peer-1")
	}
	l.Add("peer-1")
	assertPeers(t, l, "peer-1")

	l.Remove("peer-1")
	if l.Has("peer-1") {
		t.Fatal("Remove should deny peer-1")
	}
	l.Remove("peer-1")
	assertPeers(t, l)
}

func TestAllowListConcurrent(t *testing.T) {
	l, err := NewAllowList()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("p-%d", i)
			l.Add(id)
			_ = l.Has(id)
			_ = l.Peers()
			l.Remove(id)
		}(i)
	}
	wg.Wait()
	assertPeers(t, l)
}

func assertPeers(t *testing.T, l *AllowList, want ...string) {
	t.Helper()
	got := l.Peers()
	if len(got) != len(want) {
		t.Fatalf("Peers() = %v, want %v", got, want)
	}
	set := make(map[string]struct{}, len(got))
	for _, p := range got {
		set[p] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			t.Fatalf("Peers() missing %q: %v", w, got)
		}
	}
}
