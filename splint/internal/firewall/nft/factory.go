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
	hook                      string
	netns                     []string
	ingressFlags, egressFlags scopeFlags

	// The validated hook and enforcement scopes Open builds its
	// drivers with.
	hookValue Hook
	ingress   Scope
	egress    Scope
}

// scopeFlags is one direction's bound scope flags: the flag names
// travel with the values so refused-value errors can quote them.
type scopeFlags struct {
	protos, ports, addrs          []string
	protoFlag, portFlag, addrFlag string
}

var _ firewall.Factory = (*Factory)(nil)

func NewFactory() *Factory {
	return &Factory{
		hook: string(HookInput),
		ingressFlags: scopeFlags{
			protoFlag: "--nft-ingress-l4proto", portFlag: "--nft-ingress-dport", addrFlag: "--nft-ingress-daddr",
		},
		egressFlags: scopeFlags{
			protoFlag: "--nft-egress-l4proto", portFlag: "--nft-egress-sport", addrFlag: "--nft-egress-saddr",
		},
	}
}

// Name is the driver's registry name — the --firewall value that
// selects it.
func (f *Factory) Name() string { return "nftables" }

func (f *Factory) BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&f.hook, "nft-hook", string(HookInput), "enforcement hook: input or prerouting")
	// StringArray, not StringSlice: nsfs paths are never comma lists,
	// and an empty value stays visible to the validation below.
	fs.StringArrayVar(&f.netns, "nft-netns", nil, "target network namespace nsfs path (repeatable; default: the current namespace)")
	fs.StringSliceVar(&f.ingressFlags.addrs, "nft-ingress-daddr", nil, "ingress scope destination address or prefix (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.ingressFlags.ports, "nft-ingress-dport", nil, "ingress scope destination port 1-65535 (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.ingressFlags.protos, "nft-ingress-l4proto", []string{"tcp", "udp"}, "ingress scope transport protocol: tcp or udp (comma list or repeatable)")
	fs.StringSliceVar(&f.egressFlags.addrs, "nft-egress-saddr", nil, "egress scope source address or prefix (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.egressFlags.ports, "nft-egress-sport", nil, "egress scope source port 1-65535 (comma list or repeatable; default: all)")
	fs.StringSliceVar(&f.egressFlags.protos, "nft-egress-l4proto", []string{"tcp", "udp"}, "egress scope transport protocol: tcp or udp (comma list or repeatable)")
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
	ingress, err := parseScope(f.ingressFlags)
	if err != nil {
		return err
	}
	egress, err := parseScope(f.egressFlags)
	if err != nil {
		return err
	}
	f.hookValue, f.ingress, f.egress = hook, ingress, egress
	return nil
}

// parseScope resolves one direction's scope flags into its validated
// scope; refused values name their flag and the offending value.
func parseScope(flags scopeFlags) (Scope, error) {
	var scope Scope
	var err error
	if scope.L4Protos, err = parseL4Protos(flags.protoFlag, flags.protos); err != nil {
		return Scope{}, err
	}
	if scope.Ports, err = parsePorts(flags.portFlag, flags.ports); err != nil {
		return Scope{}, err
	}
	if scope.Addrs, err = parseAddrs(flags.addrFlag, flags.addrs); err != nil {
		return Scope{}, err
	}
	scope.L4Protos = dedup(scope.L4Protos)
	scope.Ports = dedup(scope.Ports)
	scope.Addrs = dedup(scope.Addrs)
	// After dedup, any overlap is ambiguous — CIDR blocks are disjoint
	// or nested — so enforcement would not match the command line.
	for i, a := range scope.Addrs {
		for _, b := range scope.Addrs[i+1:] {
			if a.Overlaps(b) {
				return Scope{}, fmt.Errorf("%s: overlapping address prefixes (got %q and %q)", flags.addrFlag, a, b)
			}
		}
	}
	return scope, nil
}

// dedup drops repeated values, keeping the first: a kernel batch
// never sees a value twice, and the operator's stated order stands.
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

// parseL4Protos accepts strict lowercase tcp or udp — the only
// protocols a ban rule can gate on.
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

// parsePorts accepts exact numbers 1–65535, no ranges.
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

// parseAddrs accepts bare addresses (counting as their host
// prefixes) or prefixes.
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
// none is configured. Opening is lazy: no I/O happens until a driver
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
