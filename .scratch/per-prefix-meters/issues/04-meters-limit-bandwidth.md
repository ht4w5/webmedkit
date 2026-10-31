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

**Status:** ready-for-agent

- [ ] Each effective Target prefix of a bandwidth-limited ban rule carries
      its own limit state on the egress direction, metering response bytes.
- [ ] The independence pin extends to bandwidth meters: saturating one
      prefix's byte budget leaves a same-rate sibling unaffected.
- [ ] Placement and direction semantics are unchanged (egress chain, the
      response direction the Penalty's unit names).
- [ ] The docs mapping table and limit prose cover both metered stances
      consistently.
