// Package nft is splint's nftables firewall driver: it enforces the
// firewall seam's ban rules through netlink
// (github.com/google/nftables) - one wholly-owned inet table named
// splint in the target namespace, rebuilt wholesale in a single
// atomic batch per apply (no delete-then-rebuild window), removed on
// reset. It applies each Penalty exactly as stated or declines it:
// limit-req-rate is application-layer intent and is never faked at
// the packet layer (ADR 0012).
//
// The driver owns its identity: the rolling hash over its effective
// rules, the skip-if-unchanged decision with its existence probe and
// the counter read-back live here.
package nft

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
)

// The wholly-owned nftables objects. The chain names are the
// placement labels the plan reports (the --dry-run mapping lines
// name these chains).
const (
	tableName   = "splint"
	ingressName = "ingress"
	egressName  = "egress"
)

// Hook is the enforcement hook the ingress chain attaches to: input
// catches locally-destined traffic, prerouting catches transit and
// local traffic.
type Hook string

// The two enforcement hooks (--hook).
const (
	HookInput      Hook = "input"
	HookPrerouting Hook = "prerouting"
)

func ParseHook(s string) (Hook, error) {
	h := Hook(s)
	if !h.valid() {
		return "", fmt.Errorf("hook: must be input or prerouting (got %q)", s)
	}
	return h, nil
}

func (h Hook) valid() bool {
	return h == HookInput || h == HookPrerouting
}

// Driver is the nftables implementation of the firewall.Driver port:
// one wholly-owned table per target namespace plus its identity -
// plan is pure, apply rebuilds the table in one atomic batch
// (skipping when the hash matches and the table exists), reset
// removes it. One Driver targets one namespace: netns is its nsfs
// handle path, "" the current namespace.
type Driver struct {
	hook                      Hook
	ingressScope, egressScope Scope
	netns                     string

	// open dials the target namespace's netlink surface: openNetlink in
	// production, the fake kernel in tests.
	open func() (kernelConn, io.Closer, error)

	// Skip-if-unchanged state: the hash last applied here; installed
	// marks a succeeded apply (the zero hash is a legal hash).
	hash      uint32
	installed bool

	// prevRules are the last install's input rules - the ruleset the
	// hit read-back reports on; without them a Meter reading's prefix
	// cannot be named.
	prevRules []firewall.BanRule
}

var _ firewall.Driver = (*Driver)(nil)

// Scope is one direction's enforcement scope: the protocols, ports and
// addresses its ban rules match at the service end - destination fields
// on ingress, source fields on egress. Empty Ports or Addrs match all;
// empty L4Protos is the tcp,udp default (unscoped bans: tcp, udp only).
// Scope folds into the ruleset hash (ADR 0018), so changing it re-enforces.
type Scope struct {
	L4Protos []byte // IP protocol numbers: tcp (6) and udp (17)
	Ports    []uint16
	Addrs    []netip.Prefix
}

func (s Scope) l4protos() []byte {
	if len(s.L4Protos) == 0 {
		return []byte{unix.IPPROTO_TCP, unix.IPPROTO_UDP}
	}
	return s.L4Protos
}

// fields encodes the scope for the ruleset hash; each group leads
// with a StringField marker so no value slides across a group boundary.
func (s Scope) fields() [][]byte {
	protos := s.l4protos()
	fields := make([][]byte, 0, len(protos)+len(s.Ports)+len(s.Addrs)+3)
	fields = append(fields, firewall.StringField("l4proto"))
	for _, p := range protos {
		fields = append(fields, firewall.Uint16Field(uint16(p)))
	}
	fields = append(fields, firewall.StringField("port"))
	for _, p := range s.Ports {
		fields = append(fields, firewall.Uint16Field(p))
	}
	fields = append(fields, firewall.StringField("addr"))
	for _, a := range s.Addrs {
		fields = append(fields, firewall.PrefixField(a))
	}
	return fields
}

// Config is one enforcement point's configuration: hook, the shared
// ingress and egress scopes, and the nsfs path ("" is the current namespace).
type Config struct {
	Hook      Hook
	Ingress   Scope
	Egress    Scope
	NetNSPath string
}

// kernelConn is the netlink surface the driver flushes through:
// *nftables.Conn in production, the fake kernel in tests.
type kernelConn interface {
	AddTable(*nftables.Table) *nftables.Table
	AddChain(*nftables.Chain) *nftables.Chain
	AddSet(*nftables.Set, []nftables.SetElement) error
	AddRule(*nftables.Rule) *nftables.Rule
	DelTable(*nftables.Table)
	Flush() error
	ListTableOfFamily(name string, family nftables.TableFamily) (*nftables.Table, error)
	ListChainsOfTableFamily(family nftables.TableFamily) ([]*nftables.Chain, error)
	GetRules(table *nftables.Table, chain *nftables.Chain) ([]*nftables.Rule, error)
}

var _ kernelConn = (*nftables.Conn)(nil)

// New builds a driver for one enforcement point. The nsfs handle re-opens on
// every apply/reset, picking up a recreated container through its stable handle.
func New(cfg Config) (*Driver, error) {
	if !cfg.Hook.valid() {
		return nil, fmt.Errorf("hook: must be input or prerouting (got %q)", cfg.Hook)
	}
	d := &Driver{hook: cfg.Hook, ingressScope: cfg.Ingress, egressScope: cfg.Egress, netns: cfg.NetNSPath}
	d.open = d.openNetlink
	return d, nil
}

// openNetlink dials netlink to this driver's target namespace (the
// current one for an empty path) through the library's namespace-fd
// option - no setns. The nsfs handle stays open until Flush returns:
// the library dials lazily at flush time and the handle is what
// keeps the namespace alive meanwhile. An unopenable handle is a
// vanished namespace: it wraps firewall.ErrUnreachable.
func (d *Driver) openNetlink() (kernelConn, io.Closer, error) {
	if d.netns == "" {
		conn, err := nftables.New()
		if err != nil {
			return nil, nil, fmt.Errorf("connect: %w", err)
		}
		return conn, nil, nil
	}
	nsFile, err := os.Open(d.netns)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: open netns %s: %w", firewall.ErrUnreachable, d.netns, err)
	}
	conn, err := nftables.New(nftables.WithNetNSFd(int(nsFile.Fd())))
	if err != nil {
		nsFile.Close()
		return nil, nil, fmt.Errorf("%w: connect netns %s: %w", firewall.ErrUnreachable, d.netns, err)
	}
	return conn, nsFile, nil
}

func ownedTable() *nftables.Table {
	return &nftables.Table{Family: nftables.TableFamilyINet, Name: tableName}
}

// chainOfTable reports whether a listed chain belongs to the given
// table: chain listings span the family, and only the owned table's
// chains hold the driver's own counters.
func chainOfTable(chain *nftables.Chain, table *nftables.Table) bool {
	return chain.Table != nil && chain.Table.Name == table.Name && chain.Table.Family == table.Family
}

// tableExists probes for the owned table: a delete of a missing
// table aborts the whole batch with ENOENT, so only a present
// table's delete may join the batch.
func tableExists(conn kernelConn, table *nftables.Table) (bool, error) {
	if _, err := conn.ListTableOfFamily(table.Name, table.Family); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Plan is the pure planning step - no I/O, no privileges: it derives
// the effective rules (the ban rules it can honour, narrowed to the
// target prefixes that reach their address families), their
// placement labels (a change of --hook is a change of effective
// rules), each direction's scope line, the ruleset hash, and the
// declines with reasons. --dry-run prints exactly this. A ban rule
// neither honourable nor declinable refuses the plan loudly.
func (d *Driver) Plan(rules []firewall.BanRule) (firewall.Plan, error) {
	plan := firewall.Plan{Scopes: []string{
		banDirection{name: ingressName, scope: d.ingressScope}.scopeLine(),
		banDirection{name: egressName, matchDst: true, scope: d.egressScope}.scopeLine(),
	}}
	for i, r := range rules {
		if r.Stance == firewall.StanceLimitReqRate {
			// limit-req-rate is application-layer intent the packet layer
			// cannot count: declined, never faked as a meter (ADR 0012).
			plan.Declined = append(plan.Declined, firewall.Decline{
				Rule:    i,
				Feature: string(r.Stance),
				Reason:  "application-layer intent; never faked at the packet layer",
			})
			continue
		}
		dir, err := d.direction(r.Stance)
		if err != nil {
			return firewall.Plan{}, fmt.Errorf("ban rule %d: %w", i, err)
		}
		if err := checkRate(r); err != nil {
			return firewall.Plan{}, fmt.Errorf("ban rule %d: %w", i, err)
		}
		fams := ruleFamilies(r, dir.scope)
		if len(fams) == 0 {
			// Declined with a reason - a vanishing ban rule would leave
			// "why isn't this banned?" unanswerable.
			plan.Declined = append(plan.Declined, firewall.Decline{
				Rule:    i,
				Feature: string(r.Stance),
				Reason:  fmt.Sprintf("the %s enforcement scope cannot reach any target address family", dir.name),
			})
			continue
		}
		effective := r
		effective.Targets = nil
		for _, f := range fams {
			effective.Targets = append(effective.Targets, f.targets...)
		}
		plan.Rules = append(plan.Rules, firewall.PlannedRule{BanRule: effective, Placement: d.placement(dir)})
	}
	plan.Hash = d.hashEffective(plan.Rules)
	return plan, nil
}

// hashEffective folds the ruleset hash: both scopes once, globally -
// a scope change re-enforces even with no effective rules - then each rule.
func (d *Driver) hashEffective(rules []firewall.PlannedRule) uint32 {
	h := firewall.NewHasher()
	h.Step(d.ingressScope.fields()...)
	h.Step(d.egressScope.fields()...)
	for _, r := range rules {
		fields := make([][]byte, 0, len(r.Targets)+3)
		for _, t := range r.Targets {
			fields = append(fields, firewall.PrefixField(t))
		}
		fields = append(fields, firewall.StringField(string(r.Stance)))
		if r.Rate != nil {
			fields = append(fields, firewall.Uint64Field(uint64(*r.Rate)))
		}
		fields = append(fields, firewall.StringField(r.Placement))
		h.Step(fields...)
	}
	return h.Sum()
}

// placement reports the ban rule's placement label - the chain it
// installs in at the hook that chain attaches to.
func (d *Driver) placement(b banDirection) string {
	hook := string(d.hook) // the ingress chain attaches at the configured hook
	if b.matchDst {
		// The egress chain meters the response direction: output in
		// input mode (local responses), forward in prerouting mode.
		hook = "output"
		if d.hook == HookPrerouting {
			hook = "forward"
		}
	}
	return b.name + "@" + hook
}

// directionOf reports the chain a stance installs in and where its
// target match looks: source on ingress, destination on egress for
// the bandwidth meter (the response direction its unit names). The
// enforcement scope mirrors the target match - the service is the
// packet's other end: destination on ingress, source on egress.
func directionOf(s firewall.Stance) (name string, matchDst bool, err error) {
	switch s {
	case firewall.StanceDrop, firewall.StanceReject, firewall.StanceLimitPktRate:
		return ingressName, false, nil
	case firewall.StanceLimitBandwidth:
		return egressName, true, nil
	}
	return "", false, fmt.Errorf("stance %q: not supported", s)
}

// banDirection pairs a ban direction with its enforcement scope and
// its direction-prefixed per-rule scope set names.
type banDirection struct {
	name     string
	matchDst bool
	scope    Scope
}

// direction resolves a stance's ban direction with its own scope -
// one direction's scope never constrains the other direction's rules.
func (d *Driver) direction(s firewall.Stance) (banDirection, error) {
	name, matchDst, err := directionOf(s)
	if err != nil {
		return banDirection{}, err
	}
	b := banDirection{name: name, matchDst: matchDst, scope: d.ingressScope}
	if matchDst {
		b.scope = d.egressScope
	}
	return b, nil
}

// targetOff is the network-header offset the target match loads: the
// source address on ingress, the destination address on egress.
func (b banDirection) targetOff(f addrFamily) uint32 {
	if b.matchDst {
		return f.dstOff
	}
	return f.srcOff
}

// scopeOff is the network-header offset the scope's addresses load:
// the service end - destination on ingress, source on egress.
func (b banDirection) scopeOff(f addrFamily) uint32 {
	if b.matchDst {
		return f.srcOff
	}
	return f.dstOff
}

// portOff is the transport-header offset the scope's ports load:
// dport on ingress, sport on egress.
func (b banDirection) portOff() uint32 {
	if b.matchDst {
		return sportOff
	}
	return dportOff
}

// scopeLine formats the direction's scope as its one --dry-run line,
// named after the flag names' tail: dport/daddr vs sport/saddr.
func (b banDirection) scopeLine() string {
	portField, addrField := "dport", "daddr"
	if b.matchDst {
		portField, addrField = "sport", "saddr"
	}
	protos := make([]string, 0, len(b.scope.l4protos()))
	for _, p := range b.scope.l4protos() {
		protos = append(protos, protoName(p))
	}
	ports := allOrList(b.scope.Ports, func(p uint16) string { return strconv.Itoa(int(p)) })
	addrs := allOrList(b.scope.Addrs, func(a netip.Prefix) string { return a.Masked().String() })
	return fmt.Sprintf("scope=%s l4proto=%s %s=%s %s=%s",
		b.name, strings.Join(protos, ","), portField, ports, addrField, addrs)
}

func allOrList[T any](values []T, name func(T) string) string {
	if len(values) == 0 {
		return "all"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, name(v))
	}
	return strings.Join(parts, ",")
}

func protoName(p byte) string {
	switch p {
	case unix.IPPROTO_TCP:
		return "tcp"
	case unix.IPPROTO_UDP:
		return "udp"
	}
	return strconv.Itoa(int(p))
}

// The per-ban-rule scope set names: il4/idp/ida (ingress) and
// el4/esp/esa (egress); the index suffix is what ruleIndex parses.
func (b banDirection) protoSet(i int) string {
	if b.matchDst {
		return fmt.Sprintf("el4_%d", i)
	}
	return fmt.Sprintf("il4_%d", i)
}

func (b banDirection) portSet(i int) string {
	if b.matchDst {
		return fmt.Sprintf("esp_%d", i)
	}
	return fmt.Sprintf("idp_%d", i)
}

func (b banDirection) addrSet(f addrFamily, i int) string {
	kind := "ida"
	if b.matchDst {
		kind = "esa"
	}
	// f.name is "v4"/"v6"; the set name wants its digit.
	return fmt.Sprintf("%s%s_%d", kind, f.name[1:], i)
}

// checkRate refuses a limit stance without a rate the kernel meter
// can honour exactly as stated - the driver never tunes a stated
// rate.
func checkRate(r firewall.BanRule) error {
	switch r.Stance {
	case firewall.StanceLimitPktRate, firewall.StanceLimitBandwidth:
		if r.Rate == nil {
			return fmt.Errorf("stance %q: rate required", r.Stance)
		}
		if *r.Rate < 1 {
			return fmt.Errorf("stance %q: rate must be at least 1 (got %d)", r.Stance, *r.Rate)
		}
	}
	return nil
}

// Apply replaces the owned table wholesale in one atomic batch - or
// skips when the plan's hash matches the ruleset it last applied and
// the owned table still exists (probed fresh every apply) - and
// reports the hash, the replaced rules' nonzero counters (the hits
// since the last change), and the declines. The table is never
// absent while the new one applies. A declined ban rule keeps its
// input index (the counter read-back indexes the replaced ruleset)
// and installs nothing.
func (d *Driver) Apply(rules []firewall.BanRule) (firewall.Outcome, error) {
	plan, err := d.Plan(rules)
	if err != nil {
		return firewall.Outcome{}, fmt.Errorf("apply: %w", err)
	}
	conn, closer, err := d.open()
	if err != nil {
		return firewall.Outcome{}, fmt.Errorf("apply: %w", err)
	}
	if closer != nil {
		// The handle stays open through Flush (see openNetlink).
		defer closer.Close()
	}
	table := ownedTable()
	// Known limit: existence only - tampering under a surviving table
	// is undetected. Fresh every apply - an unchanged ruleset must not
	// hide behind a vanished table.
	exists, err := tableExists(conn, table)
	if err != nil {
		return firewall.Outcome{}, fmt.Errorf("apply: probe owned table: %w", err)
	}
	if d.installed && plan.Hash == d.hash && exists {
		return firewall.Outcome{Skipped: true, Hash: plan.Hash, Declined: plan.Declined}, nil
	}
	var hits []firewall.Hit
	if exists {
		hits, err = ruleHits(conn, table, d.prevRules)
		if err != nil {
			return firewall.Outcome{}, fmt.Errorf("apply: read rule counters: %w", err)
		}
		conn.DelTable(table)
	}
	table = conn.AddTable(table)
	ingress := conn.AddChain(d.ingress(table))
	egress := conn.AddChain(d.egress(table))
	declined := map[int]bool{}
	for _, decline := range plan.Declined {
		declined[decline.Rule] = true
	}
	for i, r := range rules {
		if declined[i] {
			continue // a declined ban rule installs nothing, keeping its input index
		}
		if err := d.addBanRule(conn, table, ingress, egress, i, r); err != nil {
			return firewall.Outcome{}, fmt.Errorf("apply: ban rule %d: %w", i, err)
		}
	}
	if err := conn.Flush(); err != nil {
		return firewall.Outcome{}, fmt.Errorf("apply: %w", err)
	}
	d.hash, d.installed = plan.Hash, true
	d.prevRules = slices.Clone(rules)
	return firewall.Outcome{Hash: plan.Hash, Hits: hits, Declined: plan.Declined}, nil
}

// ruleRead accumulates one ban rule's counter readings as the
// read-back walks the owned table; prefixes are its Meter readings
// keyed by the effective Target prefix they meter.
type ruleRead struct {
	packets  uint64
	bytes    uint64
	prefixes map[string]*prefixRead
}

// prefixRead is one prefix's Meter reading with the coordinates its
// entries sort by: the rule's effective-target order (family, then
// slot in the family's target list).
type prefixRead struct {
	target  string
	fam     int
	slot    int
	packets uint64
	bytes   uint64
}

// ruleHits reads the owned table's per-rule counters back, one hit
// per ban rule. Every chain the owned table holds is read: a chain
// named after its ban rule (a verdict-map element slot chain) names
// the ban rule outright, a base chain's kernel rule carries its index
// in its set names - either way the readings aggregate onto the ban
// rule they count, the ruleset being replaced (prev). A metered rule's
// Meter readings also break down per effective Target prefix. Only
// nonzero readings come back.
func ruleHits(conn kernelConn, table *nftables.Table, prev []firewall.BanRule) ([]firewall.Hit, error) {
	chains, err := conn.ListChainsOfTableFamily(table.Family)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil // the chains are gone with the table
		}
		return nil, err
	}
	byRule := map[int]*ruleRead{}
	for _, chain := range chains {
		if !chainOfTable(chain, table) {
			continue // not a chain of the owned table
		}
		mapName, slot, isSlotChain := meterChainSlot(chain.Name)
		krules, err := conn.GetRules(table, chain)
		if err != nil {
			return nil, err
		}
		for _, r := range krules {
			c, ok := ruleCounter(r)
			if !ok || (c.Packets == 0 && c.Bytes == 0) {
				continue
			}
			i, ok := chainRuleIndex(chain.Name) // the chain names its ban rule
			if !ok {
				i, ok = ruleIndex(r) // a base chain's rule names its own
			}
			if !ok {
				continue // not a kernel rule this driver wrote
			}
			read := byRule[i]
			if read == nil {
				read = &ruleRead{prefixes: map[string]*prefixRead{}}
				byRule[i] = read
			}
			read.packets += c.Packets
			read.bytes += c.Bytes
			if !isSlotChain {
				continue
			}
			if target, fam, ok := meterTarget(prev, i, mapName, slot); ok {
				p := read.prefixes[target]
				if p == nil {
					p = &prefixRead{target: target, fam: fam, slot: slot}
					read.prefixes[target] = p
				}
				p.packets += c.Packets
				p.bytes += c.Bytes
			}
		}
	}
	hits := make([]firewall.Hit, 0, len(byRule))
	for i, read := range byRule {
		h := firewall.Hit{Rule: i, Packets: read.packets, Bytes: read.bytes}
		reads := make([]*prefixRead, 0, len(read.prefixes))
		for _, p := range read.prefixes {
			reads = append(reads, p)
		}
		slices.SortFunc(reads, func(a, b *prefixRead) int {
			if a.fam != b.fam {
				return a.fam - b.fam
			}
			return a.slot - b.slot
		})
		for _, p := range reads {
			h.Prefixes = append(h.Prefixes, firewall.PrefixHit{Target: p.target, Packets: p.packets, Bytes: p.bytes})
		}
		hits = append(hits, h)
	}
	slices.SortFunc(hits, func(a, b firewall.Hit) int { return a.Rule - b.Rule })
	return hits, nil
}

// ruleCounter is a kernel rule's packet/byte counter, or false when
// it carries none. Found by shape, not index: the match exprs in
// front vary with the rule's enforcement scope.
func ruleCounter(r *nftables.Rule) (*expr.Counter, bool) {
	for _, e := range r.Exprs {
		if c, ok := e.(*expr.Counter); ok {
			return c, true
		}
	}
	return nil, false
}

// ruleIndex maps a kernel rule back onto its ban rule: the index is
// baked into its set names (v4_3, idp_3, …), so the mapping
// survives restarts.
func ruleIndex(r *nftables.Rule) (int, bool) {
	for _, e := range r.Exprs {
		l, ok := e.(*expr.Lookup)
		if !ok {
			continue
		}
		if i, err := strconv.Atoi(l.SetName[strings.LastIndex(l.SetName, "_")+1:]); err == nil {
			return i, true
		}
	}
	return 0, false
}

// meterChainSlot splits an owned chain's name at its slot separator
// (v4_3.1: verdict map v4_3, slot 1); a name without a numeric slot
// segment (the base chains) names no Meter chain.
func meterChainSlot(name string) (mapName string, slot int, ok bool) {
	dot := strings.LastIndex(name, meterSlotSep)
	if dot < 0 {
		return "", 0, false
	}
	slot, err := strconv.Atoi(name[dot+1:])
	if err != nil {
		return "", 0, false
	}
	return name[:dot], slot, true
}

// chainRuleIndex reads a ban rule's index out of an owned chain's
// name: a chain named after its verdict-map element slot strips its
// trailing slot segment and takes the index its map name carries -
// the index ruleIndex reads from a kernel rule's set names. A name
// without a slot segment (the base chains) names no ban rule.
func chainRuleIndex(name string) (int, bool) {
	mapName, _, ok := meterChainSlot(name)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(mapName[strings.LastIndex(mapName, "_")+1:])
	if err != nil {
		return 0, false
	}
	return i, true
}

// meterTarget names the effective Target prefix one Meter chain
// meters, through the ruleset being replaced (prev): the chain's
// verdict map names the ban rule and address family, its slot the
// prefix's position in that family's target list. Only a metered rule
// owns Meters - anything else matches no prefix. The returned family
// position orders entries like the rule's effective targets.
func meterTarget(prev []firewall.BanRule, rule int, mapName string, slot int) (string, int, bool) {
	if rule < 0 || rule >= len(prev) {
		return "", 0, false
	}
	r := prev[rule]
	if !metered(r.Stance) {
		return "", 0, false
	}
	for fam, f := range families {
		if f.setName(rule) != mapName {
			continue
		}
		targets := familyPrefixes(r.Targets, f)
		if slot < 0 || slot >= len(targets) {
			return "", 0, false
		}
		return targets[slot].String(), fam, true
	}
	return "", 0, false
}

// Reset removes the owned table with everything in it, tolerating the
// table being already absent, and ends the skip state: after a reset
// the same ruleset installs again.
func (d *Driver) Reset() error {
	conn, closer, err := d.open()
	if err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	if closer != nil {
		// The handle stays open through Flush (see openNetlink).
		defer closer.Close()
	}
	conn.DelTable(ownedTable())
	if err := conn.Flush(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reset: %w", err)
	}
	d.installed = false
	d.prevRules = nil
	return nil
}

// ingress is the ingress base chain at the configured hook: filter
// priority at input, raw priority at prerouting so bans run before
// conntrack.
func (d *Driver) ingress(table *nftables.Table) *nftables.Chain {
	c := &nftables.Chain{
		Name:  ingressName,
		Table: table,
		Type:  nftables.ChainTypeFilter,
	}
	switch d.hook {
	case HookPrerouting:
		c.Hooknum, c.Priority = nftables.ChainHookPrerouting, nftables.ChainPriorityRaw
	default:
		c.Hooknum, c.Priority = nftables.ChainHookInput, nftables.ChainPriorityFilter
	}
	return c
}

// egress is the egress base chain carrying the bandwidth limits, at
// the response direction they meter: output in input mode (local
// responses), forward in prerouting mode (transit responses).
func (d *Driver) egress(table *nftables.Table) *nftables.Chain {
	c := &nftables.Chain{
		Name:  egressName,
		Table: table,
		Type:  nftables.ChainTypeFilter,
	}
	switch d.hook {
	case HookPrerouting:
		c.Hooknum, c.Priority = nftables.ChainHookForward, nftables.ChainPriorityFilter
	default:
		c.Hooknum, c.Priority = nftables.ChainHookOutput, nftables.ChainPriorityFilter
	}
	return c
}

// addrFamily is one address family a ban rule can cover; srcOff and
// dstOff are the network-header offsets of its addresses, srcLen the
// address length every address load shares.
type addrFamily struct {
	name    string
	proto   byte
	keyType nftables.SetDatatype
	srcOff  uint32
	srcLen  uint32
	dstOff  uint32
	is      func(netip.Prefix) bool
	setName func(i int) string
}

// The two families an inet table covers. Set names carry the ban
// rule's index (parsed back by ruleIndex).
var families = []addrFamily{
	{
		name:    "v4",
		proto:   unix.NFPROTO_IPV4,
		keyType: nftables.TypeIPAddr,
		srcOff:  12,
		srcLen:  4,
		dstOff:  16,
		is:      func(p netip.Prefix) bool { return p.Addr().Is4() },
		setName: func(i int) string { return fmt.Sprintf("v4_%d", i) },
	},
	{
		name:    "v6",
		proto:   unix.NFPROTO_IPV6,
		keyType: nftables.TypeIP6Addr,
		srcOff:  8,
		srcLen:  16,
		dstOff:  24,
		is:      func(p netip.Prefix) bool { return !p.Addr().Is4() },
		setName: func(i int) string { return fmt.Sprintf("v6_%d", i) },
	},
}

// The transport-header offsets of a TCP/UDP port: one conversation's
// two views - dport in the request direction, sport in the response.
const (
	sportOff = 0
	dportOff = 2
)

type familyRule struct {
	family     addrFamily
	targets    []netip.Prefix
	scopeAddrs []netip.Prefix
}

// ruleFamilies splits one ban rule per family the scope can reach:
// no target prefix or no matching scope addresses means the family is out.
func ruleFamilies(r firewall.BanRule, scope Scope) []familyRule {
	var fams []familyRule
	for _, fam := range families {
		targets := familyPrefixes(r.Targets, fam)
		if len(targets) == 0 {
			continue
		}
		scopeAddrs := familyPrefixes(scope.Addrs, fam)
		if len(scope.Addrs) > 0 && len(scopeAddrs) == 0 {
			continue
		}
		fams = append(fams, familyRule{fam, targets, scopeAddrs})
	}
	return fams
}

// addBanRule buffers one ban rule's sets, Meter chains and kernel
// rules: one target set (a metered stance's interval verdict map into
// its per-prefix Meters, a plain interval set otherwise) and one
// kernel rule per reachable family, each kernel rule its target match
// composed with its terminal.
func (d *Driver) addBanRule(conn kernelConn, table *nftables.Table, ingress, egress *nftables.Chain, i int, r firewall.BanRule) error {
	isMetered := metered(r.Stance)
	var terminal []expr.Any
	if !isMetered {
		var err error
		terminal, err = terminalExprs(r)
		if err != nil {
			return err
		}
	}
	dir, err := d.direction(r.Stance)
	if err != nil {
		return err
	}
	chain := ingress
	if dir.matchDst {
		chain = egress
	}
	familyRules := ruleFamilies(r, dir.scope)
	if len(familyRules) == 0 {
		return nil
	}
	// The protocol gate is always present (the tcp,udp default when
	// unscoped); gate and ports are family-independent, built once per rule.
	if err := conn.AddSet(&nftables.Set{
		Table:   table,
		KeyType: nftables.TypeInetProto,
		Name:    dir.protoSet(i),
	}, protoElements(dir.scope.l4protos())); err != nil {
		return err
	}
	scopeExprs := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Lookup{SourceRegister: 1, SetName: dir.protoSet(i)},
	}
	if len(dir.scope.Ports) > 0 {
		if err := conn.AddSet(&nftables.Set{
			Table:   table,
			KeyType: nftables.TypeInetService,
			Name:    dir.portSet(i),
		}, portElements(dir.scope.Ports)); err != nil {
			return err
		}
		// Ports are transport-header bytes: the protocol gate keeps
		// other protocols' header bytes from matching by accident.
		scopeExprs = append(scopeExprs,
			&expr.Payload{DestRegister: 1, OperationType: expr.PayloadLoad,
				Base: expr.PayloadBaseTransportHeader, Offset: dir.portOff(), Len: 2},
			&expr.Lookup{SourceRegister: 1, SetName: dir.portSet(i)},
		)
	}
	for _, f := range familyRules {
		fam := f.family
		match := []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{fam.proto}},
		}
		match = append(match, scopeExprs...)
		if len(f.scopeAddrs) > 0 {
			if err := conn.AddSet(&nftables.Set{
				Table:    table,
				KeyType:  fam.keyType,
				Name:     dir.addrSet(fam, i),
				Interval: true,
			}, intervalElements(f.scopeAddrs)); err != nil {
				return err
			}
			match = append(match,
				&expr.Payload{DestRegister: 1, OperationType: expr.PayloadLoad,
					Base: expr.PayloadBaseNetworkHeader, Offset: dir.scopeOff(fam), Len: fam.srcLen},
				&expr.Lookup{SourceRegister: 1, SetName: dir.addrSet(fam, i)},
			)
		}
		match = append(match, targetMatchExprs(dir, fam, i, isMetered)...)
		if isMetered {
			// One Meter per effective Target prefix (ADR 0020). The
			// jump ends the rule: the terminal lives in the slot chains.
			if err := addMeterChains(conn, table, fam, i, r, f.targets); err != nil {
				return err
			}
			if err := conn.AddSet(&nftables.Set{
				Table:    table,
				KeyType:  fam.keyType,
				Name:     fam.setName(i),
				Interval: true,
				IsMap:    true,
				DataType: nftables.TypeVerdict,
			}, meterElements(f.targets, fam.setName(i))); err != nil {
				return err
			}
			conn.AddRule(&nftables.Rule{Table: table, Chain: chain, Exprs: match})
			continue
		}
		if err := conn.AddSet(&nftables.Set{
			Table:    table,
			KeyType:  fam.keyType,
			Name:     fam.setName(i),
			Interval: true,
		}, intervalElements(f.targets)); err != nil {
			return err
		}
		// The target match and the terminal compose into the one
		// kernel rule: either substitutes without the other.
		conn.AddRule(&nftables.Rule{Table: table, Chain: chain, Exprs: append(match, terminal...)})
	}
	return nil
}

// targetMatchExprs builds a ban rule's target match: the load of the
// address its direction matches on and the lookup of the family's
// target set. A verdict map's lookup lands its element's jump verdict
// in the verdict register - the destination the kernel applies it
// from. It composes with terminalExprs into one kernel rule - match
// first, terminal last.
func targetMatchExprs(dir banDirection, fam addrFamily, i int, verdictMap bool) []expr.Any {
	lookup := &expr.Lookup{SourceRegister: 1, SetName: fam.setName(i)}
	if verdictMap {
		lookup.DestRegister = unix.NFT_REG_VERDICT
		lookup.IsDestRegSet = true
	}
	return []expr.Any{
		&expr.Payload{DestRegister: 1, OperationType: expr.PayloadLoad,
			Base: expr.PayloadBaseNetworkHeader, Offset: dir.targetOff(fam), Len: fam.srcLen},
		lookup,
	}
}

func familyPrefixes(prefixes []netip.Prefix, fam addrFamily) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range prefixes {
		if fam.is(p) {
			out = append(out, p)
		}
	}
	return out
}

func protoElements(protos []byte) []nftables.SetElement {
	elems := make([]nftables.SetElement, 0, len(protos))
	for _, p := range protos {
		elems = append(elems, nftables.SetElement{Key: []byte{p}})
	}
	return elems
}

// portElements encodes the scope's ports as an inet-service set's
// exact keys: two-byte big-endian port numbers, the transport
// header's own byte order.
func portElements(ports []uint16) []nftables.SetElement {
	elems := make([]nftables.SetElement, 0, len(ports))
	for _, p := range ports {
		elems = append(elems, nftables.SetElement{Key: []byte{byte(p >> 8), byte(p)}})
	}
	return elems
}

// terminalExprs builds a ban rule's terminal: the counter and the
// stance's verdict tail. The counter sits after every match and
// immediately before the verdict, so it counts only the ban rule's
// own hits.
func terminalExprs(r firewall.BanRule) ([]expr.Any, error) {
	verdict, err := verdictExprs(r)
	if err != nil {
		return nil, err
	}
	return append([]expr.Any{&expr.Counter{}}, verdict...), nil
}

// verdictExprs maps a terminal stance onto its verdict tail: drop
// and reject only (unknown stances error loudly). Reject is icmpx
// admin-prohibited: TCP peers get a reset automatically. Reject at
// prerouting needs kernel ~5.9 - older kernels refuse the batch, the
// documented floor for --hook prerouting.
func verdictExprs(r firewall.BanRule) ([]expr.Any, error) {
	switch r.Stance {
	case firewall.StanceDrop:
		return []expr.Any{&expr.Verdict{Kind: expr.VerdictDrop}}, nil
	case firewall.StanceReject:
		return []expr.Any{&expr.Reject{
			Type: unix.NFT_REJECT_ICMPX_UNREACH,
			Code: unix.NFT_REJECT_ICMPX_ADMIN_PROHIBITED,
		}}, nil
	default:
		if metered(r.Stance) {
			// An inline limit would share one budget across the prefixes.
			return nil, fmt.Errorf("stance %q: terminates in its Meter chains, never at the rule", r.Stance)
		}
		return nil, fmt.Errorf("stance %q: not supported", r.Stance)
	}
}

// metered reports whether a stance enforces through per-prefix Meters
// (ADR 0020) - the limit stances, packet rate and response bandwidth:
// its target set is an interval verdict map whose slots jump to
// per-prefix chains, instead of a plain target set with an inline
// limit.
func metered(s firewall.Stance) bool {
	return s == firewall.StanceLimitPktRate || s == firewall.StanceLimitBandwidth
}

// addMeterChains buffers one ban rule's Meter chains for a family:
// one chain per effective Target prefix, named after its verdict-map
// element slot (map.slot), each holding its prefix's counter, Meter
// and drop. The chains read no registers - recent kernels refuse a
// rule reading a register no earlier expression loads.
func addMeterChains(conn kernelConn, table *nftables.Table, fam addrFamily, i int, r firewall.BanRule, targets []netip.Prefix) error {
	meter, err := meterChainExprs(r)
	if err != nil {
		return err
	}
	for slot := range targets {
		chain := conn.AddChain(&nftables.Chain{Table: table, Name: meterChainName(fam.setName(i), slot)})
		conn.AddRule(&nftables.Rule{Table: table, Chain: chain, Exprs: meter})
	}
	return nil
}

// meterChainExprs builds a Meter chain's one kernel rule: the
// prefix's counter, its Meter - the limit expression with the stated
// rate, the derived burst, inverted semantics - and the drop. In-rate
// traffic matches no limit and falls off the chain end, back into the
// calling rule walk.
func meterChainExprs(r firewall.BanRule) ([]expr.Any, error) {
	lim, err := limitExpr(r)
	if err != nil {
		return nil, err
	}
	return []expr.Any{&expr.Counter{}, lim, &expr.Verdict{Kind: expr.VerdictDrop}}, nil
}

// limitExpr is a ban rule's rate limit as one limit expression: the
// Penalty's stated rate, the derived burst, and inverted ("over")
// semantics - only the traffic past the rate matches, so the drop
// applies to the excess alone.
func limitExpr(r firewall.BanRule) (*expr.Limit, error) {
	if err := checkRate(r); err != nil {
		return nil, err
	}
	limitType := expr.LimitTypePkts
	if r.Stance == firewall.StanceLimitBandwidth {
		limitType = expr.LimitTypePktBytes
	}
	return &expr.Limit{
		Type:  limitType,
		Rate:  uint64(*r.Rate),
		Over:  true,
		Unit:  expr.LimitTimeSecond,
		Burst: burstFor(*r.Rate),
	}, nil
}

// burstFor derives a rate meter's burst: one second's worth of the
// rate, floored to the kernel's 32-bit field. nft's tiny default
// burst would drop legitimate micro-bursts.
func burstFor(rate int64) uint32 {
	if rate > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(rate) // rate >= 1 where the compiler emits it
}

// meterSlotSep is the one separator in a Meter chain's map.slot name
// (v4_3.1): meterChainName builds the shape chainRuleIndex parses.
const meterSlotSep = "."

// meterChainName is the chain one verdict-map element jumps to: the
// map name plus the element's numeric slot suffix - the shape
// chainRuleIndex strips to name the ban rule.
func meterChainName(mapName string, slot int) string {
	return mapName + meterSlotSep + strconv.Itoa(slot)
}

// meterElements encodes the effective Target prefixes as a verdict
// map's element pairs: intervalElements' ranges, each start element
// carrying the jump into its slot's Meter chain (one element per
// prefix, slots positional in the target list).
func meterElements(prefixes []netip.Prefix, mapName string) []nftables.SetElement {
	elems := intervalElements(prefixes)
	for slot := range prefixes {
		elems[2*slot].VerdictData = &expr.Verdict{Kind: expr.VerdictJump, Chain: meterChainName(mapName, slot)}
	}
	return elems
}

// intervalElements encodes masked Target prefixes as an interval
// set's element pairs: [start, end), end flagged IntervalEnd and
// holding the first address after the range. A range reaching the top
// of the address space has no representable end, so it gets the
// kernel's null end: [start, top].
func intervalElements(prefixes []netip.Prefix) []nftables.SetElement {
	var elems []nftables.SetElement
	for _, p := range prefixes {
		p = p.Masked()
		start := p.Addr().AsSlice()
		end := prefixEnd(p)
		if end == nil {
			end = make([]byte, len(start)) // the null end
		}
		elems = append(elems,
			nftables.SetElement{Key: start},
			nftables.SetElement{Key: end, IntervalEnd: true},
		)
	}
	return elems
}

// prefixEnd returns the first address after p's range as raw key
// bytes, or nil when the range reaches the top of the address space.
func prefixEnd(p netip.Prefix) []byte {
	start := p.Addr().AsSlice()
	end := make([]byte, len(start))
	bits := p.Bits()
	for i := range end {
		// Set the byte's host bits: end holds the range's last address.
		kept := min(max(bits-8*i, 0), 8)
		end[i] = start[i] | byte(0xff>>kept)
	}
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end
		}
	}
	return nil // carried past the top of the address space
}
