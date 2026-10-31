# 01: Prefactor - target-match/terminal seam and chain-aware hit read-back

**What to build:** No behavior change. Make the metering change easy before
making it: the nftables firewall driver's rule builder composes a ban rule's
target match independently from its terminal (counter and verdict), and the
hit read-back attributes counters from any chain the driver owns by name,
not just the two base chains. Everything observable - plan, ruleset hash,
installed shape, logs, outcomes - stays identical.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A ban rule's target match and its terminal compose independently in
      the rule builder, so a stance can later substitute either without
      touching the other.
- [x] The hit read-back attributes a counter from any owned chain to its
      ban rule by chain name, aggregating per ban rule as today.
- [x] The whole existing suite passes unchanged - this refactor lands green
      with zero diff in observable behavior.

## Answer

Implemented on `ticket/ppm-01-prefactor-match-terminal-seam` (commits
`1f5e7ef` seam composition, `f4be9c3` chain-aware read-back), to be
merged into `feat/per-prefix-meters`. Pure prefactor, zero observable
behavior change.

The rule builder now composes a ban rule's target match
(`targetMatchExprs`: the target-address load and the target set
lookup) and its terminal (`terminalExprs`: the counter and the
stance's verdict tail) as separate pieces appended into one kernel
rule - the seam at which tickets 02/04 substitute the limit stances'
target match (verdict map) and terminal (the per-prefix chain's
counter/limit/drop). The hit read-back enumerates every chain the
owned table holds and attributes each counter to its ban rule: a
chain named after its verdict-map element slot (map name plus
numeric slot suffix, e.g. `v4_3.1`) strips its trailing slot segment
and names the ban rule; a base chain's kernel rule still carries its
index in its set names (`ruleIndex`, untouched). `kernelConn` lists
chains (`ListChainsOfTableFamily`; `ListChain` dropped, the driver
no longer fetches chains by name).

Context: `../../splint/internal/firewall/nft/nft.go`,
`fakekernel_test.go` (`TestApplyComposesTheTargetMatchAndTheTerminal`,
`TestApplyAttributesCountersFromAnyOwnedChain`), `netns_test.go`
(`TestKernelAttributesOwnedChainCountersByChainName`).

Notes:
- Meter-chain naming is decided here for the read-back: `<map>.<slot>`,
  e.g. `v4_3.0`, slot numeric and positional in the family's target
  list. Tickets 02/04 must name their chains exactly so (the
  read-back and its tests already parse this shape).
- The read-back prefers the chain name where the chain carries a slot
  segment, and falls back to the rule's set names otherwise; both
  mechanisms agree on every shape the driver installs.
- The root-gated suites were exercised for real in an unprivileged
  user+net namespace (uid 0 inside, `unshare --user --map-root-user
  --net`): all four kernel tests pass against a real kernel.
- Kernel note for later tickets: recent kernels reject a rule that
  reads a register no earlier expression loads (netlink errno
  ENOMSG). The meter chain's counter/limit/drop reads no registers,
  so its shape is safe.
