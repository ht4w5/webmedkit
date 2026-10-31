# 02: Per-prefix Meters for limit-pkt-rate

**What to build:** A packet-rate-limited ban rule no longer shares one
rate-limit state across its same-rate Target prefixes. The nftables firewall
driver installs one interval verdict map per (ban rule, address family),
whose elements jump to one chain per effective Target prefix; each chain
holds that prefix's counter, its limit expression (the Penalty's stated rate,
the derived burst of one second's worth, inverted "over" semantics), and the
drop. In-rate traffic falls off the chain end and returns to the rule walk,
exactly as today. Hit totals per ban rule keep reporting "since the last
change". Drop and reject ban rules are untouched. The operator documentation
describes limit enforcement as one Meter per effective Target prefix.

**Blocked by:** 01: Prefactor - target-match/terminal seam and chain-aware
hit read-back

**Status:** ready-for-agent

- [ ] Each effective Target prefix of a packet-rate-limited ban rule carries
      its own limit state: the installed read-back shows one limit instance
      per effective Target prefix, never one shared across the group.
- [ ] Each meter chain holds exactly three expressions in order: counter,
      inverted limit with stated rate and derived burst, drop.
- [ ] In-rate traffic returns to the rule walk (fall-through semantics and
      the documented first-match limitation are unchanged).
- [ ] Hit totals per ban rule are unchanged in shape and meaning ("since the
      change"), including for metered ban rules.
- [ ] Drop and reject ban rules install exactly as before.
- [ ] Fake-kernel shape tests and the root-gated install/read-back test pass.
- [ ] Operator docs use the Meter vocabulary and state the per-effective-
      Target-prefix unit.
