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

**Status:** resolved

- [x] Each effective Target prefix of a packet-rate-limited ban rule carries
      its own limit state: the installed read-back shows one limit instance
      per effective Target prefix, never one shared across the group.
- [x] Each meter chain holds exactly three expressions in order: counter,
      inverted limit with stated rate and derived burst, drop.
- [x] In-rate traffic returns to the rule walk (fall-through semantics and
      the documented first-match limitation are unchanged).
- [x] Hit totals per ban rule are unchanged in shape and meaning ("since the
      change"), including for metered ban rules.
- [x] Drop and reject ban rules install exactly as before.
- [x] Fake-kernel shape tests and the root-gated install/read-back test pass.
- [x] Operator docs use the Meter vocabulary and state the per-effective-
      Target-prefix unit.

## Answer

Implemented on `ticket/ppm-02-meters-limit-pkt-rate`. A packet-rate-limited
ban rule now installs one interval verdict map per (ban rule, address
family) - keeping the ban rule's existing `v4_<i>`/`v6_<i>` name - whose
elements jump to one Meter chain per effective Target prefix, named after
its map element slot (`<map>.<slot>`, e.g. `v4_3.1`, as ticket 01 fixed).
Each chain holds exactly its prefix's counter, its Meter (the stated rate,
the derived burst of one second's worth, inverted "over" semantics) and the
drop; in-rate traffic falls off the chain end back into the rule walk -
fall-through and the documented first-match limitation unchanged. Drop and
reject install exactly as before; limit-bandwidth keeps its inline limit
until its own ticket; limit-req-rate keeps declining with its reason. Hit
totals per ban rule are unchanged - the chain-aware read-back from ticket 01
aggregates the Meter chains' counters by chain name. Plan output, ruleset
hash and the outcome contract are untouched.

Context: `../../splint/internal/firewall/nft/nft.go` (`addBanRule`'s
metered branch; `metered`, `addMeterChains`, `meterChainExprs`, `limitExpr`,
`meterChainName`, `meterElements`), `fakekernel_test.go`
(`TestApplyInstallsOneMeterPerEffectiveTargetPrefix`), `kernel_test.go`
(`meterTail`, the metered `wantRule`, `wantMeters` in the shared shape
assertions), `docs/splint.md`.

Notes:
- The verdict-map lookup must land its element's jump verdict in the
  verdict register (`DestRegister: NFT_REG_VERDICT, IsDestRegSet`). Without
  it a real kernel (7.2 tested) applies no verdict: matching traffic just
  falls through and the Meter chain never sees a packet. `nft` CLI emits
  the same lookup; the fake-kernel shape test pins it.
- The root-gated suites were exercised against a real kernel in an
  unprivileged user+net namespace (`unshare --user --map-root-user --net`),
  all green. A throwaway traffic spike also confirmed the behavior: 20
  packets at a 5/s meter jumped to the Meter chain (counter 20), the
  excess was dropped, the in-rate 5 fell through onto the broader rule
  below (the documented first-match limitation), and the hit read-back
  reported rule 0 = 20 / rule 1 = 5 packets "since the last change".
- Kernel read-back (google/nftables v0.3.0) reports a verdict map's
  element verdicts only as raw bytes in `SetElement.Val` (and clobbers
  `KeyType` with the verdict type), so the root-gated shape assertions pin
  the map's element keys and the slot chains; the element-to-chain jump
  wiring is pinned at the fake seam.
- For ticket 04: bandwidth adopts the same machinery by widening `metered()`
  and reusing `limitExpr` (already dimension-aware). Its docs mapping-table
  row and the limit prose still describe the shared per-ban-rule limit and
  must be brought in line so "the docs mapping table and limit prose cover
  both metered stances consistently".
