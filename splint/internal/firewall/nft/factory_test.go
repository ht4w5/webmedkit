package nft

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
)

// The command run seam is deliberately untested: what would be
// asserted through the command is verified here, at the module that
// owns it.

// newTestFactory parses args through the factory's flag group the way
// the command does.
func newTestFactory(t *testing.T, args ...string) *Factory {
	t.Helper()
	f := NewFactory()
	fs := pflag.NewFlagSet("splint", pflag.ContinueOnError)
	f.BindFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return f
}

func TestFactoryValidations(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "an unknown enforcement hook is refused",
			args:    []string{"--nft-hook", "postrouting"},
			wantErr: `hook: must be input or prerouting (got "postrouting")`,
		},
		{
			name: "the prerouting enforcement hook is accepted",
			args: []string{"--nft-hook", "prerouting"},
		},
		{
			name:    "an empty namespace path is refused",
			args:    []string{"--nft-netns", ""},
			wantErr: "netns: namespace path must not be empty",
		},
		{
			name: "namespace paths are accepted",
			args: []string{"--nft-netns", "/var/run/netns/a", "--nft-netns", "/var/run/netns/b"},
		},
		{
			name:    "a duplicate namespace path is refused",
			args:    []string{"--nft-netns", "/var/run/netns/a", "--nft-netns", "/var/run/netns/a"},
			wantErr: `netns: duplicate namespace path "/var/run/netns/a"`,
		},
		{
			name: "distinct namespace paths are accepted",
			args: []string{"--nft-netns", "/var/run/netns/a", "--nft-netns", "/run/netns/b"},
		},
		{
			name: "the scope flags accept comma lists and repeats",
			args: []string{
				"--nft-ingress-daddr", "203.0.113.5,2001:db8::/64",
				"--nft-ingress-daddr", "198.51.100.5",
				"--nft-ingress-dport", "443,80",
				"--nft-ingress-dport", "22",
				"--nft-ingress-l4proto", "tcp",
				"--nft-egress-saddr", "192.0.2.5",
				"--nft-egress-sport", "53,8053",
				"--nft-egress-l4proto", "udp",
			},
		},
		{
			name: "bare addresses count as their host prefixes",
			args: []string{"--nft-ingress-daddr", "203.0.113.5", "--nft-egress-saddr", "2001:db8::5"},
		},
		{
			name: "the port boundaries are accepted",
			args: []string{"--nft-ingress-dport", "1", "--nft-egress-sport", "65535"},
		},
		{
			name: "the protocol scope defaults to tcp and udp",
		},
		{
			name:    "an uppercase protocol is refused",
			args:    []string{"--nft-ingress-l4proto", "TCP"},
			wantErr: `--nft-ingress-l4proto: must be tcp or udp (got "TCP")`,
		},
		{
			name:    "an unknown protocol is refused",
			args:    []string{"--nft-egress-l4proto", "sctp"},
			wantErr: `--nft-egress-l4proto: must be tcp or udp (got "sctp")`,
		},
		{
			name:    "an empty protocol entry is refused",
			args:    []string{"--nft-ingress-l4proto", "tcp,,"},
			wantErr: `--nft-ingress-l4proto: must be tcp or udp (got "")`,
		},
		{
			name:    "a zero ingress port is refused",
			args:    []string{"--nft-ingress-dport", "0"},
			wantErr: `--nft-ingress-dport: must be a number between 1 and 65535 (got "0")`,
		},
		{
			name:    "a port above 65535 is refused",
			args:    []string{"--nft-egress-sport", "65536"},
			wantErr: `--nft-egress-sport: must be a number between 1 and 65535 (got "65536")`,
		},
		{
			name:    "a port range is refused",
			args:    []string{"--nft-ingress-dport", "443-445"},
			wantErr: `--nft-ingress-dport: must be a number between 1 and 65535 (got "443-445")`,
		},
		{
			name:    "a garbage egress port is refused",
			args:    []string{"--nft-egress-sport", "https"},
			wantErr: `--nft-egress-sport: must be a number between 1 and 65535 (got "https")`,
		},
		{
			name:    "a garbage ingress address is refused",
			args:    []string{"--nft-ingress-daddr", "not-an-address"},
			wantErr: `--nft-ingress-daddr: unparseable prefix (got "not-an-address")`,
		},
		{
			name:    "a garbage egress address is refused",
			args:    []string{"--nft-egress-saddr", "203.0.113.256"},
			wantErr: `--nft-egress-saddr: unparseable prefix (got "203.0.113.256")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFactory(t, tt.args...)
			err := f.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestFactoryOpensItsEnforcementPoints(t *testing.T) {
	t.Run("one driver instance per namespace path, labeled with its path", func(t *testing.T) {
		f := newTestFactory(t, "--nft-netns", "/var/run/netns/a", "--nft-netns", "/run/netns/b")
		if err := f.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		points, err := f.Open()
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		var labels []string
		for _, p := range points {
			labels = append(labels, p.Label)
		}
		want := []string{"/var/run/netns/a", "/run/netns/b"}
		if strings.Join(labels, ",") != strings.Join(want, ",") {
			t.Errorf("point labels = %q, want %q", labels, want)
		}
		if points[0].Driver == points[1].Driver {
			t.Error("the points share one driver instance: skip-if-unchanged must follow each instance")
		}
	})

	t.Run("without namespace paths the single point is the current namespace", func(t *testing.T) {
		points, err := newTestFactory(t).Open()
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if len(points) != 1 || points[0].Label != "current" {
			t.Errorf("points = %+v, want the single current-namespace point", points)
		}
	})

	t.Run("a refused scope value fails before any enforcement point opens", func(t *testing.T) {
		// Validation runs before Open builds anything: a refused value
		// touches no enforcement surface.
		f := newTestFactory(t, "--nft-ingress-dport", "0")
		points, err := f.Open()
		if err == nil {
			t.Fatal("Open succeeded on a refused scope value")
		}
		if !strings.Contains(err.Error(), "--nft-ingress-dport") {
			t.Errorf("Open error %q does not name the flag", err)
		}
		if len(points) != 0 {
			t.Errorf("Open returned %d points on a refused scope value, want 0", len(points))
		}
	})

	t.Run("each instance enforces at its own target handle", func(t *testing.T) {
		// A vanished target surfaces as firewall.ErrUnreachable (the
		// update cycle warns and skips it), naming the instance's path.
		points, err := newTestFactory(t, "--nft-netns", "/nonexistent/splint-test/netns").Open()
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		_, err = points[0].Driver.Apply(nil)
		if !errors.Is(err, firewall.ErrUnreachable) {
			t.Errorf("Apply at a vanished point = %v, want a firewall.ErrUnreachable", err)
		}
		if err == nil || !strings.Contains(err.Error(), "/nonexistent/splint-test/netns") {
			t.Errorf("Apply error %v does not name the instance's target handle", err)
		}
	})
}

func TestFactoryFlagGroup(t *testing.T) {
	f := NewFactory()
	fs := pflag.NewFlagSet("splint", pflag.ContinueOnError)
	f.BindFlags(fs)
	var got []string
	fs.VisitAll(func(fl *pflag.Flag) { got = append(got, fl.Name) })
	want := []string{
		"nft-egress-l4proto", "nft-egress-saddr", "nft-egress-sport",
		"nft-hook",
		"nft-ingress-daddr", "nft-ingress-dport", "nft-ingress-l4proto",
		"nft-netns",
	}
	if !slices.Equal(got, want) {
		t.Errorf("flag group = %q, want %q", got, want)
	}
}

// The shared-scope flags are removed outright — no alias, no
// deprecation shim — so they parse as unknown flags and keep the
// refused-command-line exit.
func TestFactoryRefusesTheRemovedSharedScopeFlags(t *testing.T) {
	for _, name := range []string{"nft-service-port", "nft-service-address"} {
		t.Run(name, func(t *testing.T) {
			f := NewFactory()
			fs := pflag.NewFlagSet("splint", pflag.ContinueOnError)
			f.BindFlags(fs)
			err := fs.Parse([]string{"--" + name, "443"})
			if err == nil {
				t.Fatalf("--%s parsed, want an unknown-flag refusal", name)
			}
			if !strings.Contains(err.Error(), "unknown flag") {
				t.Errorf("parse error %q does not name the flag unknown", err)
			}
		})
	}
}

// The scope flags resolve to one effective scope per direction. The
// plan's hash — which folds both scopes — is the external witness of
// what the flag values resolved to.
func TestFactoryScopeFlagsResolveOneScopePerDirection(t *testing.T) {
	hashOf := func(t *testing.T, args ...string) uint32 {
		t.Helper()
		points, err := newTestFactory(t, args...).Open()
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		plan, err := points[0].Driver.Plan(nil)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		return plan.Hash
	}

	t.Run("comma lists and repeats accumulate into one scope", func(t *testing.T) {
		lists := hashOf(t, "--nft-ingress-dport", "443,80", "--nft-ingress-dport", "22")
		repeats := hashOf(t, "--nft-ingress-dport", "443", "--nft-ingress-dport", "80,22")
		if lists != repeats {
			t.Errorf("comma lists hash %x, repeats hash %x: both must resolve to one scope", lists, repeats)
		}
	})

	t.Run("a bare address counts as its host prefix", func(t *testing.T) {
		bare := hashOf(t, "--nft-egress-saddr", "203.0.113.5")
		prefix := hashOf(t, "--nft-egress-saddr", "203.0.113.5/32")
		if bare != prefix {
			t.Errorf("bare address hashes %x, host prefix %x: a bare address is its host prefix", bare, prefix)
		}
	})

	t.Run("the protocol scope defaults to tcp and udp", func(t *testing.T) {
		defaults := hashOf(t)
		explicit := hashOf(t, "--nft-ingress-l4proto", "tcp,udp", "--nft-egress-l4proto", "tcp,udp")
		if defaults != explicit {
			t.Errorf("defaults hash %x, explicit tcp,udp %x: the protocol scope must default to tcp and udp", defaults, explicit)
		}
		tcpOnly := hashOf(t, "--nft-ingress-l4proto", "tcp")
		udpOnly := hashOf(t, "--nft-egress-l4proto", "udp")
		if tcpOnly == defaults || udpOnly == defaults || tcpOnly == udpOnly {
			t.Errorf("scopes hash defaults=%x ingress-tcp=%x egress-udp=%x: each protocol narrowing is its own scope", defaults, tcpOnly, udpOnly)
		}
	})
}

func TestFactoryName(t *testing.T) {
	if got := NewFactory().Name(); got != "nftables" {
		t.Errorf("Name() = %q, want %q", got, "nftables")
	}
}
