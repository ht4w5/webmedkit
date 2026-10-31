package nft

// Regression pins for the install-batch netlink wire limits: the ACK
// stream overflow and the element-list attribute wrap.

import (
	"net/netip"
	"testing"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
)

// testPrefixes builds n distinct /32 Target prefixes.
func testPrefixes(n int) []netip.Prefix {
	targets := make([]netip.Prefix, n)
	for i := range targets {
		targets[i] = netip.PrefixFrom(netip.AddrFrom4([4]byte{100, byte(i >> 16), byte(i >> 8), byte(i)}), 32)
	}
	return targets
}

// The buffer cliff pin: without the socket sizing in openNetlink the
// install's ACK stream overflows at ~90 metered prefixes.
func TestApplyManyMeteredPrefixes(t *testing.T) {
	_, d, currentNSUntouched := pinHarness(t)
	defer currentNSUntouched()
	if _, err := d.Apply([]firewall.BanRule{
		{Targets: testPrefixes(120), Stance: firewall.StanceLimitPktRate, Rate: ratePtr(250)},
	}); err != nil {
		t.Fatalf("Apply with 120 metered prefixes: %v", err)
	}
}

// The element-list wire pin: one oversized NEWSETELEM corrupts on the
// wire (measured: 1179 of 3000 elements landed), silently truncating
// the target set for both the metered and the drop stances.
func TestApplyLargeTargetListLandsWhole(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stance firewall.Stance
	}{
		{"metered", firewall.StanceLimitPktRate},
		{"drop", firewall.StanceDrop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, d, currentNSUntouched := pinHarness(t)
			defer currentNSUntouched()
			const n = 3000
			rule := firewall.BanRule{Targets: testPrefixes(n), Stance: tc.stance}
			if tc.stance == firewall.StanceLimitPktRate {
				rule.Rate = ratePtr(250)
			}
			if _, err := d.Apply([]firewall.BanRule{rule}); err != nil {
				t.Fatalf("Apply with %d %s targets: %v", n, tc.stance, err)
			}
			// Every prefix's interval pair must be in the set: 2 elements each.
			conn := scratchConn(t, path)
			set, err := conn.GetSetByName(ownedTable(), "v4_0")
			if err != nil {
				t.Fatalf("get target set: %v", err)
			}
			elems, err := conn.GetSetElements(set)
			if err != nil {
				t.Fatalf("read set elements: %v", err)
			}
			if len(elems) != 2*n {
				t.Errorf("target set holds %d elements, want %d (2 per prefix) - target list corrupted on the wire", len(elems), 2*n)
			}
		})
	}
}
