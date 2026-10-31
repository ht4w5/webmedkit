# 04: Per-prefix Meters for limit-bandwidth

**What to build:** Response-bandwidth limits get the same per-prefix Meter
treatment as packet-rate limits: a bandwidth-limited ban rule installs its
verdict map and per-effective-Target-prefix chains on the egress direction
(output in input mode, forward in prerouting mode), metering response
payload bytes at the Penalty's stated rate with the same derived burst and
fall-through semantics. With this ticket, every enforceable limit Penalty
meters per effective Target prefix. The traffic harness's independence pin
is extended to the egress direction.

**Blocked by:** 03: Kernel traffic pin - Meter independence and fall-through

**Status:** resolved

- [x] Each effective Target prefix of a bandwidth-limited ban rule carries
      its own limit state on the egress direction, metering response bytes.
- [x] The independence pin extends to bandwidth meters: saturating one
      prefix's byte budget leaves a same-rate sibling unaffected.
- [x] Placement and direction semantics are unchanged (egress chain, the
      response direction the Penalty's unit names).
- [x] The docs mapping table and limit prose cover both metered stances
      consistently.

## Answer

Implemented on `ticket/ppm-04-meters-limit-bandwidth`. A
response-bandwidth-limited ban rule installs the same per-prefix Meter
machinery as limit-pkt-rate: one interval verdict map per (ban rule,
address family) - keeping the ban rule's `v4_<i>`/`v6_<i>` name - on
the egress direction (output in input mode, forward in prerouting
mode), keyed on the destination address the response direction
matches, whose elements jump to one Meter chain per effective Target
prefix (`<map>.<slot>`). Each chain holds its prefix's counter, its
byte Meter (the stated rate in bytes per second, the derived burst of
one second's worth, inverted "over" semantics) and the drop; in-rate
traffic falls off the chain end back into the rule walk. The whole
surface is one widened `metered()` over ticket 02's machinery
(`limitExpr` was already dimension-aware). Placement and direction
semantics are unchanged; drop, reject and limit-req-rate install as
before; plan output, ruleset hash, the outcome contract and hit totals
per ban rule are unchanged via the chain-aware read-back. With this
ticket every enforceable limit Penalty meters per effective Target
prefix, one Meter per prefix per dimension.

Coverage: the fake-kernel shape pins (`TestApplyInstallsOneMeterPerEffectiveTargetPrefix`
now spanning both stances, the metered `wantRule`s,
`meterTail(LimitTypePktBytes, ...)`) and the root-gated fixtures
(`assertUnscopedKernelState`/`assertScopedKernelState`: the egress rules
end at their verdict-map lookups, `v4_1`/`v6_1`/`v4_3` are interval
verdict maps, the slot chains carry the byte-meter tails) extend to
bandwidth meters. Ticket 03's traffic harness is extended - not forked -
with `TestKernelSaturatingOneByteMeterLeavesItsSiblingUnaffected`: one
bandwidth-limited ban rule over two same-rate prefixes at the output
hook, a 20-datagram spike at one prefix, then the sibling's whole burst
while the first Meter is still dry; the read-back wants 25/15
fall-through (both Meters counted everything, only the in-budget
datagrams fell through), a shared budget reads 25/10. The docs mapping
row and limit prose describe both metered stances with the Meter
vocabulary (one Meter per effective Target prefix per dimension).

Notes:
- The kernel stocks a byte Meter with its derived burst plus one
  second's worth of tokens - 290 bytes at 145 bytes/s, exactly ten
  29-byte datagrams - and costs each packet its bytes (probed against
  the real kernel before pinning the spike arithmetic; the packet
  Meter stocks its burst and costs one packet). The pin's exact counts
  hold the boundary semantics too: the tenth datagram of the stock
  passes.
- A datagram dropped at the egress chain fails the sending `sendto`
  with EPERM - the output hook is synchronous to the send, unlike the
  input hook's drops - and is counted by the Meter's counter before
  the drop. The harness tolerates that one errno; anything else is a
  hard failure.
- The independence pin was validated red against both regression
  shapes: the old inline shared limit (read 25/10) and a throwaway
  "every map element jumps to one chain" mutation (read 25/10 and
  25/5 on the packet pin), then green; 15 repeat runs (5 with -race)
  are stable.
- Root-gated suites were exercised for real in an unprivileged
  user+net namespace (`unshare --user --map-root-user --net`), all
  green; `make test` green.
- For later (not this ticket): `verdictExprs`'s limit branch (limit +
  drop) is now unreachable through `terminalExprs`, since both limit
  stances are metered and build the same tail inside their chains. It
  is kept as the stance mapping's total tail definition; folding
  `meterChainExprs` through it would remove the duplication.
