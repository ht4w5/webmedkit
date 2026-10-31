package nft

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
)

// TestKernelInstallReadBackReset is skipped where root or nftables
// is absent, and refuses to run where a splint table already exists:
// it rebuilds and resets that table, which would drop a live
// deployment's bans.
func TestKernelInstallReadBackReset(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: netlink requires CAP_NET_ADMIN")
	}
	if reason := netlinkUnusable(); reason != "" {
		t.Skip(reason)
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
	// Whatever the test does halfway, the owned table must not outlive it.
	t.Cleanup(func() {
		d, err := New(Config{Hook: HookInput})
		if err != nil {
			t.Errorf("New: %v", err)
			return
		}
		if err := d.Reset(); err != nil {
			t.Errorf("cleanup reset: %v", err)
		}
	})

	// Phase 1: the unscoped encoding. The 255.255.255.0/24 drop
	// exercises the null-end encoding of a top-touching range.
	rules := []firewall.BanRule{
		{Targets: []netip.Prefix{
			netip.MustParsePrefix("203.0.113.0/24"),
			netip.MustParsePrefix("198.51.100.7/32"),
		}, Stance: firewall.StanceDrop},
		{Targets: []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}, Stance: firewall.StanceReject},
		{Targets: []netip.Prefix{netip.MustParsePrefix("198.51.100.9/32")}, Stance: firewall.StanceLimitPktRate, Rate: ratePtr(250)},
		{Targets: []netip.Prefix{netip.MustParsePrefix("203.0.113.9/32")}, Stance: firewall.StanceLimitBandwidth, Rate: ratePtr(1000000)},
		{Targets: []netip.Prefix{netip.MustParsePrefix("255.255.255.0/24")}, Stance: firewall.StanceDrop},
	}
	input, err := New(Config{Hook: HookInput})
	if err != nil {
		t.Fatalf("New(input): %v", err)
	}
	first, err := input.Apply(rules)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(first.Hits) != 0 {
		t.Errorf("first Apply reported %d hits, want none (nothing was installed to count)", len(first.Hits))
	}
	if exists, err := tableExists(conn, ownedTable()); err != nil || !exists {
		t.Errorf("table exists after Apply = (%v, %v), want (true, nil)", exists, err)
	}
	assertUnscopedKernelState(t, conn, HookInput)

	// Phase 2: the per-direction scoped encoding at prerouting,
	// rebuilding over the live table. Hit counts vary: only the mapping is asserted.
	ingressScope := Scope{
		L4Protos: []byte{unix.IPPROTO_TCP},
		Ports:    []uint16{443, 8443},
		Addrs: []netip.Prefix{
			netip.MustParsePrefix("198.51.100.5/32"),
			netip.MustParsePrefix("2001:db8::5/128"),
		},
	}
	egressScope := Scope{
		L4Protos: []byte{unix.IPPROTO_UDP},
		Ports:    []uint16{53},
		Addrs: []netip.Prefix{
			netip.MustParsePrefix("192.0.2.5/32"),
			netip.MustParsePrefix("2001:db8:1::5/128"),
		},
	}
	scopedRules := []firewall.BanRule{
		{
			Targets: []netip.Prefix{
				netip.MustParsePrefix("203.0.113.7/32"),
				netip.MustParsePrefix("2001:db8::7/128"),
			},
			Stance: firewall.StanceDrop,
		},
		{
			Targets: []netip.Prefix{
				netip.MustParsePrefix("203.0.113.9/32"),
				netip.MustParsePrefix("2001:db8::9/128"),
			},
			Stance: firewall.StanceLimitBandwidth,
			Rate:   ratePtr(1000000),
		},
	}
	prerouting, err := New(Config{Hook: HookPrerouting, Ingress: ingressScope, Egress: egressScope})
	if err != nil {
		t.Fatalf("New(prerouting): %v", err)
	}
	rebuild, err := prerouting.Apply(scopedRules)
	if err != nil {
		t.Fatalf("rebuild Apply: %v", err)
	}
	for _, h := range rebuild.Hits {
		if h.Rule < 0 || h.Rule >= len(rules) {
			t.Errorf("counter read-back mapped to ban rule %d, want 0..%d (the replaced rules)", h.Rule, len(rules)-1)
		}
	}
	assertScopedKernelState(t, conn, HookPrerouting)

	// Phase 3: the rule's only family has no service traffic in the
	// ingress scope, so it gets no kernel rule at all.
	v4Only, err := New(Config{Hook: HookInput, Ingress: Scope{
		Ports: []uint16{443},
		Addrs: []netip.Prefix{netip.MustParsePrefix("198.51.100.5/32")},
	}})
	if err != nil {
		t.Fatalf("New(v4-only scope): %v", err)
	}
	if _, err := v4Only.Apply([]firewall.BanRule{{
		Targets: []netip.Prefix{netip.MustParsePrefix("2001:db8::9/128")},
		Stance:  firewall.StanceDrop,
	}}); err != nil {
		t.Fatalf("unreachable-family Apply: %v", err)
	}
	assertEmptyKernelState(t, conn)

	if err := prerouting.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if exists, err := tableExists(conn, ownedTable()); err != nil || exists {
		t.Errorf("table exists after Reset = (%v, %v), want (false, nil)", exists, err)
	}
	if exists, err := tableExists(conn, ownedTable()); err != nil {
		t.Fatalf("probe after Reset: %v", err)
	} else if exists {
		t.Errorf("table %q still exists after Reset", tableName)
	}
	if err := prerouting.Reset(); err != nil {
		t.Errorf("second Reset: %v", err)
	}
}

func TestKernelDeclinesInstallNothing(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: netlink requires CAP_NET_ADMIN")
	}
	if reason := netlinkUnusable(); reason != "" {
		t.Skip(reason)
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
			t.Errorf("New: %v", err)
			return
		}
		if err := d.Reset(); err != nil {
			t.Errorf("cleanup reset: %v", err)
		}
	})

	rules := []firewall.BanRule{
		{Targets: []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")}, Stance: firewall.StanceDrop},
		{Targets: []netip.Prefix{netip.MustParsePrefix("198.51.100.9/32")}, Stance: firewall.StanceLimitReqRate, Rate: ratePtr(300)},
	}
	d, err := New(Config{Hook: HookInput})
	if err != nil {
		t.Fatalf("New(input): %v", err)
	}
	out, err := d.Apply(rules)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(out.Declined) != 1 {
		t.Fatalf("Install reported %d declines, want 1: %+v", len(out.Declined), out.Declined)
	}
	if got := out.Declined[0]; got.Rule != 1 || got.Feature != "limit-req-rate" || got.Reason == "" {
		t.Errorf("Declined = %+v, want rule 1's limit-req-rate with a reason", got)
	}

	table, err := conn.ListTableOfFamily(tableName, nftables.TableFamilyINet)
	if err != nil {
		t.Fatalf("read table back: %v", err)
	}
	for chainName, want := range map[string]int{ingressName: 1, egressName: 0} {
		chain, err := conn.ListChain(table, chainName)
		if err != nil {
			t.Fatalf("read chain %s back: %v", chainName, err)
		}
		krules, err := conn.GetRules(table, chain)
		if err != nil {
			t.Fatalf("read rules of %s back: %v", chainName, err)
		}
		if len(krules) != want {
			t.Errorf("chain %s carries %d kernel rules, want %d (the declined rule installs nothing)", chainName, len(krules), want)
		}
	}

	if err := d.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
}

func ratePtr(n int64) *int64 { return &n }

// assertUnscopedKernelState reads the owned table back and asserts
// the unscoped fixture's encoding. Expectations are hand-written
// from the fixtures, never computed the way the implementation does.
func assertUnscopedKernelState(t *testing.T, conn *nftables.Conn, hook Hook) {
	t.Helper()
	// The kernel rules, in install order.
	ingressRules := []wantRule{
		{proto: unix.NFPROTO_IPV4, off: 12, length: 4, setName: "v4_0", l4Set: "il4_0", tail: dropTail},   // 203.0.113.0/24 + 198.51.100.7/32
		{proto: unix.NFPROTO_IPV6, off: 8, length: 16, setName: "v6_1", l4Set: "il4_1", tail: rejectTail}, // 2001:db8::/32
		{proto: unix.NFPROTO_IPV4, off: 12, length: 4, setName: "v4_2", l4Set: "il4_2", metered: true},    // 198.51.100.9/32, one Meter per prefix
		{proto: unix.NFPROTO_IPV4, off: 12, length: 4, setName: "v4_4", l4Set: "il4_4", tail: dropTail},   // 255.255.255.0/24
	}
	egressRules := []wantRule{
		{proto: unix.NFPROTO_IPV4, off: 16, length: 4, setName: "v4_3", l4Set: "el4_3", metered: true}, // 203.0.113.9/32, one Meter per prefix on the egress direction
	}
	// The sets behind the rules; v4_4's top-touching range gets the
	// null end (its "end 0.0.0.0" element). Every rule's protocol gate
	// holds the unscoped ban's tcp and udp default.
	wantSets := map[string]wantSet{
		"v4_0": {interval: true, elems: []string{"end 198.51.100.8", "end 203.0.114.0", "start 198.51.100.7", "start 203.0.113.0"}},
		"v6_1": {interval: true, elems: []string{"end 2001:db9::", "start 2001:db8::"}},
		// v4_2 and v4_3 are the metered rules' verdict maps. The
		// read-back reports their element keys; each element's jump into
		// its slot's Meter chain is pinned at the fake seam.
		"v4_2":  {isMap: true, interval: true, elems: []string{"end 198.51.100.10", "start 198.51.100.9"}},
		"v4_3":  {isMap: true, interval: true, elems: []string{"end 203.0.113.10", "start 203.0.113.9"}},
		"v4_4":  {interval: true, elems: []string{"end 0.0.0.0", "start 255.255.255.0"}},
		"il4_0": {elems: []string{"17", "6"}}, // udp and tcp; sorted as strings
		"il4_1": {elems: []string{"17", "6"}},
		"il4_2": {elems: []string{"17", "6"}},
		"il4_4": {elems: []string{"17", "6"}},
		"el4_3": {elems: []string{"17", "6"}},
	}
	wantMeters := map[string]tailCheck{
		"v4_2.0": meterTail(expr.LimitTypePkts, 250, 250),
		"v4_3.0": meterTail(expr.LimitTypePktBytes, 1000000, 1000000),
	}
	assertKernelState(t, conn, hook, ingressRules, egressRules, wantSets, wantMeters)
}

// assertScopedKernelState reads the owned table back and asserts the
// scoped fixture's encoding: each rule under its direction's
// enforcement scope, the two scopes' values distinct, in both families.
func assertScopedKernelState(t *testing.T, conn *nftables.Conn, hook Hook) {
	t.Helper()
	ingressRules := []wantRule{
		{
			proto: unix.NFPROTO_IPV4, off: 12, length: 4, setName: "v4_0",
			l4Set: "il4_0", portSet: "idp_0", portOff: 2,
			scopeSet: "ida4_0", scopeOff: 16, scopeLen: 4,
			tail: dropTail, // 203.0.113.7/32 under the ingress scope
		},
		{
			proto: unix.NFPROTO_IPV6, off: 8, length: 16, setName: "v6_0",
			l4Set: "il4_0", portSet: "idp_0", portOff: 2,
			scopeSet: "ida6_0", scopeOff: 24, scopeLen: 16,
			tail: dropTail, // 2001:db8::7/128 under the ingress scope
		},
	}
	egressRules := []wantRule{
		{
			proto: unix.NFPROTO_IPV4, off: 16, length: 4, setName: "v4_1",
			l4Set: "el4_1", portSet: "esp_1", portOff: 0,
			scopeSet: "esa4_1", scopeOff: 12, scopeLen: 4,
			metered: true, // 203.0.113.9/32 under the egress scope
		},
		{
			proto: unix.NFPROTO_IPV6, off: 24, length: 16, setName: "v6_1",
			l4Set: "el4_1", portSet: "esp_1", portOff: 0,
			scopeSet: "esa6_1", scopeOff: 8, scopeLen: 16,
			metered: true, // 2001:db8::9/128 under the egress scope
		},
	}
	wantSets := map[string]wantSet{
		"v4_0":   {interval: true, elems: []string{"end 203.0.113.8", "start 203.0.113.7"}},
		"v6_0":   {interval: true, elems: []string{"end 2001:db8::8", "start 2001:db8::7"}},
		"v4_1":   {isMap: true, interval: true, elems: []string{"end 203.0.113.10", "start 203.0.113.9"}},
		"v6_1":   {isMap: true, interval: true, elems: []string{"end 2001:db8::a", "start 2001:db8::9"}},
		"ida4_0": {interval: true, elems: []string{"end 198.51.100.6", "start 198.51.100.5"}},
		"ida6_0": {interval: true, elems: []string{"end 2001:db8::6", "start 2001:db8::5"}},
		"esa4_1": {interval: true, elems: []string{"end 192.0.2.6", "start 192.0.2.5"}},
		"esa6_1": {interval: true, elems: []string{"end 2001:db8:1::6", "start 2001:db8:1::5"}},
		"il4_0":  {elems: []string{"6"}},           // the ingress scope gates on tcp alone
		"idp_0":  {elems: []string{"443", "8443"}}, // the ingress scope's dports; sorted as strings
		"el4_1":  {elems: []string{"17"}},          // the egress scope gates on udp alone
		"esp_1":  {elems: []string{"53"}},          // the egress scope's sport
	}
	wantMeters := map[string]tailCheck{
		"v4_1.0": meterTail(expr.LimitTypePktBytes, 1000000, 1000000),
		"v6_1.0": meterTail(expr.LimitTypePktBytes, 1000000, 1000000),
	}
	assertKernelState(t, conn, hook, ingressRules, egressRules, wantSets, wantMeters)
}

// assertEmptyKernelState asserts a rebuild that installed nothing:
// both chains carry no kernel rules and there are no sets at all.
func assertEmptyKernelState(t *testing.T, conn *nftables.Conn) {
	t.Helper()
	table, err := conn.ListTableOfFamily(tableName, nftables.TableFamilyINet)
	if err != nil {
		t.Fatalf("read table back: %v", err)
	}
	for _, name := range []string{ingressName, egressName} {
		chain, err := conn.ListChain(table, name)
		if err != nil {
			t.Fatalf("read chain %s back: %v", name, err)
		}
		krules, err := conn.GetRules(table, chain)
		if err != nil {
			t.Fatalf("read rules of %s back: %v", name, err)
		}
		if len(krules) != 0 {
			t.Errorf("chain %s carries %d kernel rules, want 0 (the unreachable ban rule installs nothing)", name, len(krules))
		}
	}
	sets, err := conn.GetSets(table)
	if err != nil {
		t.Fatalf("read sets back: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("kernel sets = %d, want 0 (the unreachable ban rule installs nothing)", len(sets))
	}
}

// assertKernelState asserts the shared encoding shape the wrappers
// above hand-write their wants for: chains at their hooks, each
// wanted rule's match order ending in its verdict tail, the sets, and
// the Meter chains (one per effective Target prefix, nothing else).
func assertKernelState(t *testing.T, conn *nftables.Conn, hook Hook, ingressRules, egressRules []wantRule, wantSets map[string]wantSet, wantMeters map[string]tailCheck) {
	t.Helper()
	table, err := conn.ListTableOfFamily(tableName, nftables.TableFamilyINet)
	if err != nil {
		t.Fatalf("read table back: %v", err)
	}
	if table.Name != tableName || table.Family != nftables.TableFamilyINet {
		t.Errorf("table = (%q, %v), want (%q, inet)", table.Name, table.Family, tableName)
	}

	// Egress sits where the response direction flows: output in input
	// mode, forward in prerouting mode.
	ingressHook, ingressPrio := nftables.ChainHookInput, nftables.ChainPriorityFilter
	egressHook := nftables.ChainHookOutput
	if hook == HookPrerouting {
		ingressHook, ingressPrio = nftables.ChainHookPrerouting, nftables.ChainPriorityRaw
		egressHook = nftables.ChainHookForward
	}
	ingress := assertChain(t, conn, table, ingressName, ingressHook, ingressPrio)
	egress := assertChain(t, conn, table, egressName, egressHook, nftables.ChainPriorityFilter)

	assertRules := func(t *testing.T, chain *nftables.Chain, want []wantRule) {
		t.Helper()
		krules, err := conn.GetRules(table, chain)
		if err != nil {
			t.Fatalf("read %s rules back: %v", chain.Name, err)
		}
		if len(krules) != len(want) {
			t.Fatalf("chain %s rules = %d, want %d: %v", chain.Name, len(krules), len(want), krules)
		}
		for i, w := range want {
			assertRule(t, krules[i], w)
		}
	}
	assertRules(t, ingress, ingressRules)
	assertRules(t, egress, egressRules)

	// The chains: the two base chains plus one Meter chain per
	// effective Target prefix - no chain this fixture did not name.
	chains, err := conn.ListChainsOfTableFamily(nftables.TableFamilyINet)
	if err != nil {
		t.Fatalf("read chains back: %v", err)
	}
	chainByName := map[string]*nftables.Chain{}
	var gotChains []string
	for _, c := range chains {
		if c.Table == nil || c.Table.Name != tableName || c.Table.Family != nftables.TableFamilyINet {
			continue // not a chain of the owned table
		}
		gotChains = append(gotChains, c.Name)
		chainByName[c.Name] = c
	}
	slices.Sort(gotChains)
	wantChains := []string{egressName, ingressName}
	for name := range wantMeters {
		wantChains = append(wantChains, name)
	}
	slices.Sort(wantChains)
	if !slices.Equal(gotChains, wantChains) {
		t.Errorf("chains = %v, want %v (one Meter chain per effective Target prefix, no more)", gotChains, wantChains)
	}
	for name, tail := range wantMeters {
		chain, ok := chainByName[name]
		if !ok {
			continue // its absence is named above
		}
		if chain.Hooknum != nil {
			t.Errorf("chain %s hooks at %d, want a plain Meter chain", name, *chain.Hooknum)
		}
		krules, err := conn.GetRules(table, chain)
		if err != nil {
			t.Fatalf("read %s rules back: %v", name, err)
		}
		if len(krules) != 1 {
			t.Fatalf("chain %s carries %d kernel rules, want exactly 1 (the prefix's counter, Meter and drop)", name, len(krules))
		}
		tail(t, krules[0].Exprs)
	}

	sets, err := conn.GetSets(table)
	if err != nil {
		t.Fatalf("read sets back: %v", err)
	}
	if len(sets) != len(wantSets) {
		t.Fatalf("kernel sets = %d, want %d", len(sets), len(wantSets))
	}
	for _, s := range sets {
		w, ok := wantSets[s.Name]
		if !ok {
			t.Errorf("unexpected kernel set %q", s.Name)
			continue
		}
		if s.IsMap != w.isMap {
			t.Errorf("set %q map = %v, want %v", s.Name, s.IsMap, w.isMap)
		}
		if s.Interval != w.interval {
			t.Errorf("set %q interval = %v, want %v", s.Name, s.Interval, w.interval)
		}
		elems, err := conn.GetSetElements(s)
		if err != nil {
			t.Fatalf("read set %q elements: %v", s.Name, err)
		}
		got := elementStrings(s, elems)
		if !slices.Equal(got, w.elems) {
			t.Errorf("set %q elements = %v, want %v", s.Name, got, w.elems)
		}
	}
}

// wantRule is one expected kernel rule: the match prefix in order -
// family gate, scope (protocol gate, ports, addresses), target match -
// then its terminal. A metered rule's target set is its verdict map:
// the map lookup ends the rule and its counter and verdict live in the
// map's slot chains (wantMeters).
type wantRule struct {
	proto   byte
	off     uint32 // the target-match load: source on ingress, destination on egress
	length  uint32
	setName string

	l4Set    string // the protocol gate set
	portSet  string // non-empty: the scope's port set
	portOff  uint32 // the scope's port load: dport (2) on ingress, sport (0) on egress
	scopeSet string // non-empty: the scope's address set
	scopeOff uint32
	scopeLen uint32

	metered bool // the target set is the ban rule's verdict map
	tail    tailCheck
}

// wantSet is one expected kernel set: map and interval flags and
// elements, already sorted as strings.
type wantSet struct {
	isMap    bool
	interval bool
	elems    []string
}

func assertChain(t *testing.T, conn *nftables.Conn, table *nftables.Table, name string, hook *nftables.ChainHook, prio *nftables.ChainPriority) *nftables.Chain {
	t.Helper()
	chain, err := conn.ListChain(table, name)
	if err != nil {
		t.Fatalf("read %s chain back: %v", name, err)
	}
	if chain.Type != nftables.ChainTypeFilter {
		t.Errorf("chain %s type = %q, want %q", name, chain.Type, nftables.ChainTypeFilter)
	}
	if chain.Hooknum == nil || *chain.Hooknum != *hook {
		t.Errorf("chain %s hook = %v, want %v", name, chain.Hooknum, *hook)
	}
	if chain.Priority == nil || *chain.Priority != *prio {
		t.Errorf("chain %s priority = %v, want %v", name, chain.Priority, *prio)
	}
	return chain
}

// tailCheck asserts a kernel rule's tail: its verdict expressions, or
// the counter and verdict of a Meter chain's one rule.
type tailCheck func(t *testing.T, exprs []expr.Any)

// assertRule asserts one kernel rule end to end: the match prefix in
// wantRule's order, then the terminal - the counter and the verdict
// tail, or for a metered rule the bare verdict-map lookup the rule
// ends on.
func assertRule(t *testing.T, r *nftables.Rule, w wantRule) {
	t.Helper()
	exprs := r.Exprs
	wantLen := 6 // family gate, protocol gate, target match
	if w.portSet != "" {
		wantLen += 2
	}
	if w.scopeSet != "" {
		wantLen += 2
	}
	if !w.metered {
		wantLen++ // the counter before the verdict
	}
	if len(exprs) < wantLen {
		t.Fatalf("rule exprs = %d, want at least %d: %v", len(exprs), wantLen, exprs)
	}
	assertMeta(t, "family gate", exprs[0], expr.MetaKeyNFPROTO)
	assertCmp(t, "family gate", exprs[1], []byte{w.proto})
	assertMeta(t, "protocol gate", exprs[2], expr.MetaKeyL4PROTO)
	assertLookup(t, "protocol set", exprs[3], w.l4Set)
	i := 4
	if w.portSet != "" {
		assertPayload(t, "scope-port load", exprs[i], expr.PayloadBaseTransportHeader, w.portOff, 2)
		i++
		assertLookup(t, "scope-port set", exprs[i], w.portSet)
		i++
	}
	if w.scopeSet != "" {
		assertPayload(t, "scope-address load", exprs[i], expr.PayloadBaseNetworkHeader, w.scopeOff, w.scopeLen)
		i++
		assertLookup(t, "scope-address set", exprs[i], w.scopeSet)
		i++
	}
	assertPayload(t, "target-address load", exprs[i], expr.PayloadBaseNetworkHeader, w.off, w.length)
	i++
	assertLookup(t, "target set", exprs[i], w.setName)
	if w.metered {
		// The verdict map's lookup lands its element's jump verdict in
		// the verdict register; the jump ends the rule, so no counter
		// may follow the lookup.
		if l, ok := exprs[i].(*expr.Lookup); !ok || !l.IsDestRegSet || l.DestRegister != unix.NFT_REG_VERDICT {
			t.Errorf("map lookup = %+v, want the verdict-map lookup into the verdict register", exprs[i])
		}
		if len(exprs) != i+1 {
			t.Errorf("rule exprs = %d, want %d (a metered rule ends at its verdict-map lookup): %v", len(exprs), i+1, exprs)
		}
		return
	}
	i++
	if _, ok := exprs[i].(*expr.Counter); !ok {
		t.Errorf("rule counter = %+v, want the packet/byte counter before the verdict", exprs[i])
	}
	i++
	w.tail(t, exprs[i:])
}

func dropTail(t *testing.T, exprs []expr.Any) {
	t.Helper()
	if len(exprs) != 1 {
		t.Fatalf("verdict exprs = %d, want 1: %v", len(exprs), exprs)
	}
	verdict, ok := exprs[0].(*expr.Verdict)
	if !ok || verdict.Kind != expr.VerdictDrop {
		t.Errorf("verdict = %+v, want the drop verdict", exprs[0])
	}
}

func rejectTail(t *testing.T, exprs []expr.Any) {
	t.Helper()
	if len(exprs) != 1 {
		t.Fatalf("verdict exprs = %d, want 1: %v", len(exprs), exprs)
	}
	rej, ok := exprs[0].(*expr.Reject)
	if !ok || rej.Type != unix.NFT_REJECT_ICMPX_UNREACH || rej.Code != unix.NFT_REJECT_ICMPX_ADMIN_PROHIBITED {
		t.Errorf("verdict = %+v, want reject with icmpx admin-prohibited", exprs[0])
	}
}

// meterTail asserts a Meter chain's one kernel rule: the prefix's
// counter, then its Meter and the drop - the three expressions, in
// order, and nothing more.
func meterTail(limitType expr.LimitType, rate uint64, burst uint32) tailCheck {
	return func(t *testing.T, exprs []expr.Any) {
		t.Helper()
		if len(exprs) != 3 {
			t.Fatalf("meter chain rule exprs = %d, want 3: %v", len(exprs), exprs)
		}
		if _, ok := exprs[0].(*expr.Counter); !ok {
			t.Errorf("meter counter = %+v, want the packet/byte counter first", exprs[0])
		}
		limitTail(limitType, rate, burst)(t, exprs[1:])
	}
}

func limitTail(limitType expr.LimitType, rate uint64, burst uint32) tailCheck {
	return func(t *testing.T, exprs []expr.Any) {
		t.Helper()
		if len(exprs) != 2 {
			t.Fatalf("verdict exprs = %d, want 2: %v", len(exprs), exprs)
		}
		lim, ok := exprs[0].(*expr.Limit)
		if !ok || lim.Type != limitType || lim.Rate != rate || !lim.Over ||
			lim.Unit != expr.LimitTimeSecond || lim.Burst != burst {
			t.Errorf("meter = %+v, want limit rate over %d/second burst %d", exprs[0], rate, burst)
		}
		verdict, ok := exprs[1].(*expr.Verdict)
		if !ok || verdict.Kind != expr.VerdictDrop {
			t.Errorf("verdict = %+v, want the drop verdict", exprs[1])
		}

	}
}

func assertMeta(t *testing.T, what string, e expr.Any, key expr.MetaKey) {
	t.Helper()
	meta, ok := e.(*expr.Meta)
	if !ok || meta.Key != key || meta.Register != 1 {
		t.Errorf("%s = %+v, want the meta load into register 1", what, e)
	}
}

func assertCmp(t *testing.T, what string, e expr.Any, data []byte) {
	t.Helper()
	cmp, ok := e.(*expr.Cmp)
	if !ok || cmp.Op != expr.CmpOpEq || cmp.Register != 1 || !bytes.Equal(cmp.Data, data) {
		t.Errorf("%s = %+v, want the register-1 comparison against %v", what, e, data)
	}
}

func assertPayload(t *testing.T, what string, e expr.Any, base expr.PayloadBase, off, length uint32) {
	t.Helper()
	payload, ok := e.(*expr.Payload)
	if !ok || payload.Base != base || payload.Offset != off || payload.Len != length ||
		payload.DestRegister != 1 || payload.OperationType != expr.PayloadLoad {
		t.Errorf("%s = %+v, want the %d:%d load into register 1", what, e, off, length)
	}
}

func assertLookup(t *testing.T, what string, e expr.Any, setName string) {
	t.Helper()
	lookup, ok := e.(*expr.Lookup)
	if !ok || lookup.SourceRegister != 1 || lookup.SetName != setName {
		t.Errorf("%s = %+v, want the lookup of %q from register 1", what, e, setName)
	}
}

// elementStrings renders one set's elements as sorted strings for
// the hand-written expectations.
func elementStrings(s *nftables.Set, elems []nftables.SetElement) []string {
	var got []string
	for _, e := range elems {
		switch s.KeyType {
		case nftables.TypeInetService:
			got = append(got, fmt.Sprintf("%d", binary.BigEndian.Uint16(e.Key)))
		case nftables.TypeInetProto:
			got = append(got, fmt.Sprintf("%d", e.Key[0]))
		default:
			kind := "start"
			if e.IntervalEnd {
				kind = "end"
			}
			entry := fmt.Sprintf("%s %s", kind, net.IP(e.Key))
			if e.VerdictData != nil {
				// A verdict map's element: the jump into its slot's chain.
				entry += fmt.Sprintf(" -> jump %s", e.VerdictData.Chain)
			}
			got = append(got, entry)
		}
	}
	slices.Sort(got)
	return got
}

// netlinkUnusable reports why the kernel nftables surface cannot be
// tested here ("" when it can); shared by the root gate and the
// loud-failure test's inverse gate.
func netlinkUnusable() string {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Sprintf("netlink: %v", err)
	}
	if _, err := conn.ListTablesOfFamily(nftables.TableFamilyINet); err != nil {
		return fmt.Sprintf("nftables: %v", err)
	}
	return ""
}
