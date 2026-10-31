# 01: Prefactor - target-match/terminal seam and chain-aware hit read-back

**What to build:** No behavior change. Make the metering change easy before
making it: the nftables firewall driver's rule builder composes a ban rule's
target match independently from its terminal (counter and verdict), and the
hit read-back attributes counters from any chain the driver owns by name,
not just the two base chains. Everything observable - plan, ruleset hash,
installed shape, logs, outcomes - stays identical.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A ban rule's target match and its terminal compose independently in
      the rule builder, so a stance can later substitute either without
      touching the other.
- [ ] The hit read-back attributes a counter from any owned chain to its
      ban rule by chain name, aggregating per ban rule as today.
- [ ] The whole existing suite passes unchanged - this refactor lands green
      with zero diff in observable behavior.
