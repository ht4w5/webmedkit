package nft

// Regression pin for the install-batch netlink buffer cliffs: the batch's
// per-message ACKs overflow the socket without the sizing in openNetlink.

import (
	"net/netip"
	"os"
	"testing"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
	"github.com/google/nftables"
)

func TestApplyManyMeteredPrefixes(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: netlink requires CAP_NET_ADMIN")
	}
	conn, err := nftables.New()
	if err != nil {
		t.Fatalf("nftables.New: %v", err)
	}
	if exists, err := tableExists(conn, ownedTable()); err != nil {
		t.Fatalf("probe owned table: %v", err)
	} else if exists {
		t.Skipf("table %q exists: a splint instance may be enforcing here", tableName)
	}
	t.Cleanup(func() {
		d, err := New(Config{Hook: HookInput})
		if err != nil {
			return
		}
		_ = d.Reset()
	})

	const n = 120
	targets := make([]netip.Prefix, n)
	for i := range targets {
		targets[i] = netip.PrefixFrom(netip.AddrFrom4([4]byte{100, byte(i >> 16), byte(i >> 8), byte(i)}), 32)
	}
	d, err := New(Config{Hook: HookInput})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := d.Apply([]firewall.BanRule{
		{Targets: targets, Stance: firewall.StanceLimitPktRate, Rate: ratePtr(250)},
	}); err != nil {
		t.Fatalf("Apply with %d metered prefixes: %v", n, err)
	}
}

// Regression pin for the element-list wire limit: one oversized
// NEWSETELEM corrupts on the wire, silently truncating the target set.
func TestApplyLargeTargetListLandsWhole(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: netlink requires CAP_NET_ADMIN")
	}
	conn, err := nftables.New()
	if err != nil {
		t.Fatalf("nftables.New: %v", err)
	}
	if exists, err := tableExists(conn, ownedTable()); err != nil {
		t.Fatalf("probe owned table: %v", err)
	} else if exists {
		t.Skipf("table %q exists: a splint instance may be enforcing here", tableName)
	}
	t.Cleanup(func() {
		d, err := New(Config{Hook: HookInput})
		if err != nil {
			return
		}
		_ = d.Reset()
	})

	const n = 1500
	targets := make([]netip.Prefix, n)
	for i := range targets {
		targets[i] = netip.PrefixFrom(netip.AddrFrom4([4]byte{100, byte(i >> 16), byte(i >> 8), byte(i)}), 32)
	}
	d, err := New(Config{Hook: HookInput})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := d.Apply([]firewall.BanRule{
		{Targets: targets, Stance: firewall.StanceLimitPktRate, Rate: ratePtr(250)},
	}); err != nil {
		t.Fatalf("Apply with %d metered prefixes: %v", n, err)
	}
	// Every prefix's interval pair must be in the map: 2 elements each.
	set, err := conn.GetSetByName(ownedTable(), "v4_0")
	if err != nil {
		t.Fatalf("get map: %v", err)
	}
	elems, err := conn.GetSetElements(set)
	if err != nil {
		t.Fatalf("read map elements: %v", err)
	}
	if len(elems) != 2*n {
		t.Errorf("map holds %d elements, want %d (2 per prefix) - target list corrupted on the wire", len(elems), 2*n)
	}
}
