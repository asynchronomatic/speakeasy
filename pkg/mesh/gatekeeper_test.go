package mesh

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestPeerAllowListPinAndMembers(t *testing.T) {
	a := NewPeerAllowList()
	a.Pin("relay-1", "self")
	if !a.Has("relay-1") || !a.Has("self") {
		t.Fatal("pinned ids should be allowed")
	}
	if a.Has("stranger") {
		t.Fatal("unknown id allowed")
	}

	a.ReplaceMembers([]string{"peer-a", "peer-b"})
	if !a.Has("peer-a") || !a.Has("relay-1") {
		t.Fatal("members + pins")
	}

	a.ReplaceMembers([]string{"peer-c"})
	if a.Has("peer-a") {
		t.Fatal("replaced member still allowed")
	}
	if !a.Has("relay-1") || !a.Has("self") {
		t.Fatal("pins dropped on replace")
	}
	if !a.Has("peer-c") {
		t.Fatal("new member missing")
	}
}

func TestPeerAllowListRefreshOnMiss(t *testing.T) {
	a := NewPeerAllowList()
	calls := 0
	a.SetRefresher(func() []string {
		calls++
		return []string{"peer-new"}
	})
	a.minInterval = time.Millisecond

	if !a.Has("peer-new") {
		t.Fatal("refresh-on-miss should allow")
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
	if !a.Has("peer-new") {
		t.Fatal("cached member")
	}
	if calls != 1 {
		t.Fatalf("hot path should not refresh, calls %d", calls)
	}
}

func TestPeerAllowListRefreshCooldown(t *testing.T) {
	a := NewPeerAllowList()
	calls := 0
	a.SetRefresher(func() []string {
		calls++
		return []string{"peer-new"}
	})
	a.minInterval = time.Hour
	if a.Has("nope") {
		t.Fatal("unknown should deny")
	}
	if calls != 1 {
		t.Fatalf("first miss should refresh, calls %d", calls)
	}
	if a.Has("still-nope") {
		t.Fatal("cooldown should not pick up new ids")
	}
	if calls != 1 {
		t.Fatalf("cooldown broken, calls %d", calls)
	}
}

func TestGateKeeperAllowsHolePunchButNotUnknownPeer(t *testing.T) {
	member := peer.ID("member-1")
	stranger := peer.ID("stranger")
	a := NewPeerAllowList()
	a.ReplaceMembers([]string{member.String()})
	g := NewGateKeeper(a)

	if !g.InterceptAccept(nil) {
		t.Fatal("accept must stay open for hole punch")
	}
	if !g.InterceptAddrDial("", nil) {
		t.Fatal("addr dial must stay open for hole punch")
	}
	if !g.InterceptPeerDial(member) {
		t.Fatal("member dial denied")
	}
	if g.InterceptPeerDial(stranger) {
		t.Fatal("stranger dial allowed")
	}
	if !g.InterceptSecured(0, member, nil) {
		t.Fatal("member secured denied")
	}
	if g.InterceptSecured(0, stranger, nil) {
		t.Fatal("stranger secured allowed")
	}
}
