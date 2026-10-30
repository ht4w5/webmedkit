package nft

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"

	flag "github.com/spf13/pflag"
	"golang.org/x/sys/unix"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
)

// Factory is the nftables firewall driver's registration: its name,
// its --nft-* flag group, its startup validation, and the opening of
// its enforcement points. Targeting and enforcement scope are driver
// configuration, not engine flags (ADR 0012), so this factory binds
// and opens its own surface.
type Factory struct {
	// The bound flag values, resolved by Validate.
	hook          string
	netns         []string
	ingressAddrs  []string
	ingressPorts  []string
	ingressProtos []string
	egressAddrs   []string
	egressPorts   []string
	egressProtos  []string

	// The validated hook and enforcement scopes Open builds its
	// drivers with.
	hookValue Hook
	ingress   Scope
	egress    Scope
}

var _ firewall.Factory = (*Factory)(nil)

func NewFactory() *Factory {
	return &Factory{hook: string(HookInput)}
}

// Name is the driver's registry name — the --firewall value that
// selects it.
func (f *Factory) Name() string { return "nftables" }

func (f *Factory) BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&f.hook, "nft-hook", string(HookInput), "enforcement hook: input or prerouting")
	// StringArray, not StringSlice: nsfs paths are never comma lists,
	// and an empty value stays visible to the validation below.
	fs.StringArrayVar(&f.netns, "nft-netns", nil, "target network namespace nsfs path (repeatable; default: the current namespace)")
	// The two enforcement scopes name the packet fields their
	// direction matches: the ingress scope sees the service at the
	// destination fields, the egress scope at the source fields. All
	// values take comma lists or repeats; protocols default to
	// tcp,udp (ADR 0018).
	fs.StringSliceVar(&f.ingressAddrs, "nft-ingress-daddr", nil, "ingress scope destination address or prefix (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.ingressPorts, "nft-ingress-dport", nil, "ingress scope destination port 1-65535 (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.ingressProtos, "nft-ingress-l4proto", []string{"tcp", "udp"}, "ingress scope transport protocol: tcp or udp (comma list or repeatable)")
	fs.StringSliceVar(&f.egressAddrs, "nft-egress-saddr", nil, "egress scope source address or prefix (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.egressPorts, "nft-egress-sport", nil, "egress scope source port 1-65535 (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.egressProtos, "nft-egress-l4proto", []string{"tcp", "udp"}, "egress scope transport protocol: tcp or udp (comma list or repeatable)")
}

func (f *Factory) Validate() error {
	hook, err := ParseHook(f.hook)
	if err != nil {
		return err
	}
	for i, path := range f.netns {
		if path == "" {
			return fmt.Errorf("netns: namespace path must not be empty (value %d)", i+1)
		}
		if slices.Contains(f.netns[:i], path) {
			return fmt.Errorf("netns: duplicate namespace path %q", path)
		}
	}
	ingress, err := parseScope(f.ingressProtos, f.ingressPorts, f.ingressAddrs,
		"--nft-ingress-l4proto", "--nft-ingress-dport", "--nft-ingress-daddr")
	if err != nil {
		return err
	}
	egress, err := parseScope(f.egressProtos, f.egressPorts, f.egressAddrs,
		"--nft-egress-l4proto", "--nft-egress-sport", "--nft-egress-saddr")
	if err != nil {
		return err
	}
	f.hookValue, f.ingress, f.egress = hook, ingress, egress
	return nil
}

// parseScope resolves one direction's scope flag values into its
// validated enforcement scope. Duplicate values resolve to one entry;
// overlapping address prefixes are refused as ambiguous. Refused
// values fail before anything is enforced, each error naming its flag
// and the offending value.
func parseScope(protos, ports, addrs []string, protoFlag, portFlag, addrFlag string) (Scope, error) {
	var scope Scope
	var err error
	if scope.L4Protos, err = parseL4Protos(protoFlag, protos); err != nil {
		return Scope{}, err
	}
	if scope.Ports, err = parsePorts(portFlag, ports); err != nil {
		return Scope{}, err
	}
	if scope.Addrs, err = parseAddrs(addrFlag, addrs); err != nil {
		return Scope{}, err
	}
	// Duplicate values — within one comma list, across repeats, or a
	// bare address and its host prefix — resolve to one scope entry,
	// silently: a kernel batch must never see a value twice (ADR 0018).
	scope.L4Protos = dedup(scope.L4Protos)
	scope.Ports = dedup(scope.Ports)
	scope.Addrs = dedup(scope.Addrs)
	// Overlapping address prefixes within one scope are ambiguous: one
	// covers part of the other's traffic, and the enforcement outcome
	// would not be what the command line states. Equal prefixes were
	// deduplicated above; CIDR blocks are either disjoint or nested,
	// so any remaining overlap is a mistake to refuse before anything
	// enforces.
	for i, a := range scope.Addrs {
		for _, b := range scope.Addrs[i+1:] {
			if a.Overlaps(b) {
				return Scope{}, fmt.Errorf("%s: overlapping address prefixes (got %q and %q)", addrFlag, a, b)
			}
		}
	}
	return scope, nil
}

// dedup drops repeated values, keeping each value's first occurrence:
// a scope is a set of values, and its resolved entries stay in the
// order the operator stated them.
func dedup[T comparable](values []T) []T {
	seen := make(map[T]struct{}, len(values))
	out := make([]T, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// parseL4Protos parses a protocol scope: strict lowercase tcp or udp,
// the only protocols a ban rule can gate on.
func parseL4Protos(flagName string, values []string) ([]byte, error) {
	protos := make([]byte, 0, len(values))
	for _, v := range values {
		switch v {
		case "tcp":
			protos = append(protos, unix.IPPROTO_TCP)
		case "udp":
			protos = append(protos, unix.IPPROTO_UDP)
		default:
			return nil, fmt.Errorf("%s: must be tcp or udp (got %q)", flagName, v)
		}
	}
	return protos, nil
}

// parsePorts parses a port scope: exact numbers 1–65535, no ranges.
func parsePorts(flagName string, values []string) ([]uint16, error) {
	ports := make([]uint16, 0, len(values))
	for _, v := range values {
		n, err := strconv.ParseUint(v, 10, 16)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%s: must be a number between 1 and 65535 (got %q)", flagName, v)
		}
		ports = append(ports, uint16(n))
	}
	return ports, nil
}

// parseAddrs parses an address scope: bare addresses (counting as
// their host prefixes) or prefixes.
func parseAddrs(flagName string, values []string) ([]netip.Prefix, error) {
	addrs := make([]netip.Prefix, 0, len(values))
	for _, v := range values {
		p, err := firewall.ParsePrefixOrAddr(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", flagName, err)
		}
		addrs = append(addrs, p)
	}
	return addrs, nil
}

// Open expands the configured targeting into one driver instance per
// enforcement point — one per nsfs path, the current namespace when
// none is configured. Every enforcement point of the instance shares
// the one scope pair. Opening is lazy: no I/O happens until a driver
// method runs, so --dry-run needs no privileges.
func (f *Factory) Open() ([]firewall.Point, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	paths := f.netns
	if len(paths) == 0 {
		paths = []string{""}
	}
	points := make([]firewall.Point, 0, len(paths))
	for _, path := range paths {
		d, err := New(Config{Hook: f.hookValue, Ingress: f.ingress, Egress: f.egress, NetNSPath: path})
		if err != nil {
			return nil, err
		}
		points = append(points, firewall.Point{Label: pointLabel(path), Driver: d})
	}
	return points, nil
}

// pointLabel is the enforcement point's opaque log label: the nsfs
// handle path, "current" for the default target. The engine logs it
// and never parses it.
func pointLabel(path string) string {
	if path == "" {
		return "current"
	}
	return path
}
