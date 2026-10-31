package nft

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"testing"

	"github.com/google/nftables"
	"golang.org/x/sys/unix"

	"github.com/HT4w5/webmedkit/splint/internal/firewall"
)

// The traffic pin: real packets through a real kernel's enforcement
// hook, read back through the same Apply outcome operators see. It is
// the repo's first traffic-driving test, and like the other namespace
// tests it is safe next to a live deployment: everything - installs,
// packets, counters - happens in the scratch namespace, which is
// born empty and dies with the test; the current namespace is only
// ever probed.

// meterRate is the pin's stated Meter rate: one second's derived
// burst (5 datagrams) is overrun by a 20-datagram spike in a
// millisecond of sends, and a spike of exactly 5 stays in rate.
const meterRate = 5

// datagramBytes is one pin datagram as the input hook counts it: the
// 20-byte IPv4 header, the 8-byte UDP header, a 1-byte payload.
const datagramBytes = 29

// TestKernelSaturatingOneMeterLeavesItsSiblingUnaffected pins the
// per-prefix Meters' independence - the regression pin against
// silent budget sharing creeping back in. One packet-rate-limited
// ban rule covers two same-rate prefixes: saturating one prefix's
// Meter drops only its excess, and the sibling's in-rate traffic
// passes untouched, as if the first prefix had never been seen.
func TestKernelSaturatingOneMeterLeavesItsSiblingUnaffected(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: netlink requires CAP_NET_ADMIN")
	}
	if reason := netlinkUnusable(); reason != "" {
		t.Skip(reason)
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("needs unshare(1) to create the scratch network namespace")
	}
	path, _ := startScratchNetNS(t, unshare)

	// Whatever happens in the scratch namespace, the current one must
	// come out exactly as it went in - possibly under a live splint.
	conn, err := nftables.New()
	if err != nil {
		t.Fatalf("nftables.New: %v", err)
	}
	existsBefore, err := tableExists(conn, ownedTable())
	if err != nil {
		t.Fatalf("probe current namespace: %v", err)
	}

	d, err := New(Config{Hook: HookInput, NetNSPath: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Reset(); err != nil {
			t.Errorf("cleanup reset: %v", err)
		}
	})
	// One packet-rate-limited ban rule over two same-rate sibling
	// prefixes - the shape whose Meters must never share budget - and
	// the broader drop below. The drop rule is the fall-through
	// witness (the documented first-match limitation): its counter
	// shows exactly what the Meters let through.
	rules := []firewall.BanRule{
		{Targets: []netip.Prefix{
			netip.MustParsePrefix("203.0.113.7/32"),
			netip.MustParsePrefix("198.51.100.7/32"),
		}, Stance: firewall.StanceLimitPktRate, Rate: ratePtr(meterRate)},
		dropFixture("0.0.0.0/0"),
	}
	if _, err := d.Apply(rules); err != nil {
		t.Fatalf("Apply into the scratch namespace: %v", err)
	}

	// Saturate the first prefix's Meter, then send the sibling's whole
	// burst back-to-back while the first prefix's Meter is still dry:
	// a shared budget would swallow the sibling's datagrams together
	// with the first prefix's excess.
	driveSpikes(t, path, []spike{
		{from: netip.MustParseAddr("203.0.113.7"), datagrams: 20},
		{from: netip.MustParseAddr("198.51.100.7"), datagrams: meterRate},
	})

	// A changed ruleset replaces the table and reports the replaced
	// rules' counters - the read-back operators see.
	out, err := d.Apply([]firewall.BanRule{dropFixture("203.0.113.8/32")})
	if err != nil {
		t.Fatalf("rebuild Apply: %v", err)
	}
	// Both Meters counted every datagram sent at them (20 + 5), but
	// only the in-rate datagrams fell off the Meter chains' ends onto
	// the drop below: the saturated prefix's burst (5 of 20) and the
	// sibling's whole burst (5 of 5). A shared budget would read
	// 25/5 - the sibling's datagrams swallowed with the excess.
	want := []firewall.Hit{
		{Rule: 0, Packets: 25, Bytes: 25 * datagramBytes},
		{Rule: 1, Packets: 10, Bytes: 10 * datagramBytes},
	}
	if !reflect.DeepEqual(out.Hits, want) {
		t.Errorf("hits = %+v, want %+v (per-prefix Meters: only the excess dropped, the sibling untouched)", out.Hits, want)
	}

	existsAfter, err := tableExists(conn, ownedTable())
	if err != nil {
		t.Fatalf("probe current namespace: %v", err)
	}
	if existsAfter != existsBefore {
		t.Errorf("current namespace table existence = %v, want %v (driving traffic elsewhere must not touch it)", existsAfter, existsBefore)
	}
}

// TestKernelInRateMeteredTrafficFallsThroughUnchanged pins the
// fall-through semantics under real traffic: a datagram within its
// prefix's Meter rate is not dropped at the Meter - it returns to the
// rule walk unchanged and meets the next rule, exactly as the inline
// limit's in-rate traffic always did.
func TestKernelInRateMeteredTrafficFallsThroughUnchanged(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: netlink requires CAP_NET_ADMIN")
	}
	if reason := netlinkUnusable(); reason != "" {
		t.Skip(reason)
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("needs unshare(1) to create the scratch network namespace")
	}
	path, _ := startScratchNetNS(t, unshare)

	conn, err := nftables.New()
	if err != nil {
		t.Fatalf("nftables.New: %v", err)
	}
	existsBefore, err := tableExists(conn, ownedTable())
	if err != nil {
		t.Fatalf("probe current namespace: %v", err)
	}

	d, err := New(Config{Hook: HookInput, NetNSPath: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Reset(); err != nil {
			t.Errorf("cleanup reset: %v", err)
		}
	})
	rules := []firewall.BanRule{
		{Targets: []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")},
			Stance: firewall.StanceLimitPktRate, Rate: ratePtr(meterRate)},
		dropFixture("0.0.0.0/0"),
	}
	if _, err := d.Apply(rules); err != nil {
		t.Fatalf("Apply into the scratch namespace: %v", err)
	}

	// A whole burst, in rate - nothing here may be dropped at the
	// Meter.
	driveSpikes(t, path, []spike{
		{from: netip.MustParseAddr("203.0.113.7"), datagrams: meterRate},
	})

	out, err := d.Apply([]firewall.BanRule{dropFixture("203.0.113.8/32")})
	if err != nil {
		t.Fatalf("rebuild Apply: %v", err)
	}
	// The Meter counted all 5 and dropped none of them: all 5 fell off
	// the Meter chain's end, back into the rule walk, and were counted
	// at the drop below. A Meter that swallowed its in-rate traffic
	// would read 5/0.
	want := []firewall.Hit{
		{Rule: 0, Packets: 5, Bytes: 5 * datagramBytes},
		{Rule: 1, Packets: 5, Bytes: 5 * datagramBytes},
	}
	if !reflect.DeepEqual(out.Hits, want) {
		t.Errorf("hits = %+v, want %+v (in-rate traffic falls through to the next rule)", out.Hits, want)
	}

	existsAfter, err := tableExists(conn, ownedTable())
	if err != nil {
		t.Fatalf("probe current namespace: %v", err)
	}
	if existsAfter != existsBefore {
		t.Errorf("current namespace table existence = %v, want %v (driving traffic elsewhere must not touch it)", existsAfter, existsBefore)
	}
}

// spike is one burst of datagrams from one source prefix: the pin's
// traffic pattern, delivered in order.
type spike struct {
	from      netip.Addr
	datagrams int
}

// driveSpikes delivers each spike back-to-back from inside the
// scratch namespace: lo up carrying the spikes' sources, one one-byte
// UDP datagram per send toward a receiver that absorbs them silently -
// a reply would be traffic of its own at the hooks below the Meters.
func driveSpikes(t *testing.T, path string, spikes []spike) {
	t.Helper()
	inScratchNetNS(t, path, func() {
		addrs := []netip.Addr{netip.MustParseAddr("127.0.0.1")}
		for _, s := range spikes {
			addrs = append(addrs, s.from)
		}
		prepareLoopback(t, addrs)
		dst := listenLoopback(t)
		for _, s := range spikes {
			sendSpike(t, dst, s)
		}
	})
}

// inScratchNetNS runs fn with the calling thread inside the scratch
// namespace, then restores the current one. Sockets and packets
// follow the thread's namespace, so the traffic cannot leave the
// scratch namespace.
func inScratchNetNS(t *testing.T, path string, fn func()) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	self, err := os.Open("/proc/self/ns/net")
	if err != nil {
		t.Fatalf("open the current namespace handle: %v", err)
	}
	defer self.Close()
	nsFile, err := os.Open(path)
	if err != nil {
		t.Fatalf("open scratch namespace handle: %v", err)
	}
	defer nsFile.Close()
	if err := unix.Setns(int(nsFile.Fd()), unix.CLONE_NEWNET); err != nil {
		t.Fatalf("enter the scratch namespace: %v", err)
	}
	defer func() {
		if err := unix.Setns(int(self.Fd()), unix.CLONE_NEWNET); err != nil {
			t.Errorf("return to the current namespace: %v", err)
		}
	}()
	fn()
}

// prepareLoopback brings lo up carrying the given addresses: the
// first is the interface's primary address, the rest land as address
// aliases (lo:1, ...) - one address per name is all SIOCSIFADDR
// gives. Local sources are what makes plain UDP sockets send the
// pin's datagrams with the prefixes' addresses on the wire.
func prepareLoopback(t *testing.T, addrs []netip.Addr) {
	t.Helper()
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open an interface-control socket: %v", err)
	}
	defer unix.Close(fd)
	for i, a := range addrs {
		name := "lo"
		if i > 0 {
			name = fmt.Sprintf("lo:%d", i)
		}
		ifr, err := unix.NewIfreq(name)
		if err != nil {
			t.Fatalf("ifreq %s: %v", name, err)
		}
		if err := ifr.SetInet4Addr(a.AsSlice()); err != nil {
			t.Fatalf("ifreq %s address: %v", name, err)
		}
		if err := unix.IoctlIfreq(fd, unix.SIOCSIFADDR, ifr); err != nil {
			t.Fatalf("assign %s to %s: %v", a, name, err)
		}
	}
	ifr, err := unix.NewIfreq("lo")
	if err != nil {
		t.Fatalf("ifreq lo: %v", err)
	}
	ifr.SetUint16(unix.IFF_UP | unix.IFF_LOOPBACK)
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, ifr); err != nil {
		t.Fatalf("bring lo up: %v", err)
	}
}

// listenLoopback opens the receiver the pin's datagrams are sent to:
// a silent UDP socket on lo. Absorbing the traffic keeps the hooks
// quiet but for the pin's own datagrams.
func listenLoopback(t *testing.T) *net.UDPAddr {
	t.Helper()
	recv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("open the receiver: %v", err)
	}
	t.Cleanup(func() { recv.Close() })
	return recv.LocalAddr().(*net.UDPAddr)
}

// sendSpike delivers one spike's datagrams as fast as the kernel
// takes them: a Meter's token bucket refills at the stated rate, so
// the sends must outrun the refill for the excess count to be exact.
func sendSpike(t *testing.T, dst *net.UDPAddr, s spike) {
	t.Helper()
	from, err := net.ListenUDP("udp4", &net.UDPAddr{IP: s.from.AsSlice()})
	if err != nil {
		t.Fatalf("open the sender for %s: %v", s.from, err)
	}
	defer from.Close()
	for i := 0; i < s.datagrams; i++ {
		if _, err := from.WriteToUDP([]byte{0x5a}, dst); err != nil {
			t.Fatalf("send from %s: %v", s.from, err)
		}
	}
}
