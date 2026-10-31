# 03: Kernel traffic pin - Meter independence and fall-through

**What to build:** Behavioral proof against a real kernel that per-prefix
Meters really are independent - the regression pin against silent budget
sharing creeping back. A scratch-namespace harness drives real packets
through the ingress hook: traffic saturating one limited prefix at its
stated rate gets its excess dropped while a same-rate sibling prefix is
unaffected, and in-rate traffic of both prefixes falls through as today.
This is the repo's first traffic-driving kernel test; it must stay safe next
to a live deployment like the existing namespace tests.

**Blocked by:** 02: Per-prefix Meters for limit-pkt-rate

**Status:** resolved

- [x] A root-gated test drives packets through the enforcement hook of a
      scratch namespace (no test can touch a live deployment's state).
- [x] Saturating one prefix's Meter drops its excess while a same-rate
      sibling prefix's traffic passes unaffected.
- [x] In-rate traffic of a metered prefix falls through unchanged.
- [x] The pin is read through the same hit/counter read-back operators use,
      so it exercises the full path, not just kernel internals.

## Answer

Implemented on `ticket/ppm-03-meter-independence-traffic-pin` (commit
`ec00362`), to be merged into `feat/per-prefix-meters`. Test-only: no
production code changed - no seam blocked the pin.

Two root-gated tests in
`../../splint/internal/firewall/nft/traffic_test.go` drive real UDP
datagrams through the input hook of a scratch namespace and read the
result through the operator's read-back (`Apply` a changed ruleset →
`Outcome.Hits`). Both install one packet-rate-limited ban rule over
effective Target prefixes plus the broader drop below - the documented
first-match limitation doubles as the fall-through witness: its counter
shows exactly what the Meters let through.

- `TestKernelSaturatingOneMeterLeavesItsSiblingUnaffected`: one ban
  rule, two same-rate prefixes (rate 5/s, derived burst 5). A
  20-datagram spike at one prefix, then the sibling's whole burst
  back-to-back while the first Meter is still dry. Hits read
  rule 0 = 25 / rule 1 = 10: both Meters counted every datagram sent at
  them, only the in-rate datagrams fell through (the saturated prefix's
  burst 5 of 20, the sibling's 5 of 5). A shared budget reads 25/5.
- `TestKernelInRateMeteredTrafficFallsThroughUnchanged`: a whole burst
  (5) at one prefix reads rule 0 = 5 / rule 1 = 5 - nothing dropped at
  the Meter, everything back in the rule walk. A Meter swallowing its
  in-rate traffic reads 5/0.

Both pins were validated red against throwaway mutations (all map
elements jumping to one chain = shared budget; `Over: false` = in-rate
swallowed), then green again. 15 repeat runs (5 with `-race`) are
stable.

Mechanics (the repo's first traffic-driving kernel test):

- The scratch namespace comes from `startScratchNetNS`; the current
  namespace is probed before/after like the other namespace tests, and
  installs only ever target the scratch one.
- The traffic runs on a `runtime.LockOSThread`ed goroutine that
  `unix.Setns`es into the nsfs handle and restores the namespace
  afterwards - no `nsenter`/`ip`/`ping` dependency. Lo is brought up
  with `SIOCSIFADDR` (sources as `lo:1`, `lo:2` aliases - one primary
  address per name) and the datagrams go from ordinary UDP sockets
  bound to the prefixes' addresses into a silent receiver on 127.0.0.1.
- Each datagram is one byte of payload: 29 bytes at the input hook
  (20 IPv4 + 8 UDP + 1), pinned in `datagramBytes`.
- Timing: the spikes must outrun the Meter's refill (one token per
  200 ms at 5/s) - 20 sends span well under a millisecond; exact
  counts held in 15 repeat runs.

For ticket 04 (bandwidth meters, egress direction): the harness is
direction-agnostic as built - `driveSpikes` sends toward a receiver and
`inScratchNetNS`/`prepareLoopback` are reusable as-is. An egress pin
needs the destination addresses instead (the bandwidth stance matches
the destination on the egress/output chain), so the receiver sockets'
addresses become the metered prefixes and the sender is one neutral
local source. Expect the same counter arithmetic at the egress chain's
Meter chains; the byte counts differ (same 29-byte datagrams if the
payload stays one byte). Note `datagramBytes` and the spike helpers
live in `traffic_test.go` next to the pin that owns them - extend, do
not fork, when adding the bandwidth pin.
