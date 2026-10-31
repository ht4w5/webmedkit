# Per-prefix meters are verdict-map chains in the nftables driver

Each limited Target prefix must own its own Meter - one rate-limit state per
effective Target prefix - but a `limit` expression's state lives per
expression instance, and the compiler packs same-rate prefixes into one ban
rule. One rule with one limit therefore shared one token bucket across every
packed prefix. We now install, for each limit ban rule and address family,
one interval verdict map whose elements jump to a small private chain per
effective Target prefix; each chain holds that prefix's counter, its limit
expression (stated rate, derived burst, inverted semantics), and the drop.

## Considered Options

- **One kernel rule per limited prefix** (each rule carries its own limit
  instance): simplest, but every packet walks the rule chain linearly - the
  worst case is benign traffic - and the O(log n) interval lookup is lost.
  Rejected at expected scale (hundreds of prefixes).
- **Dynamic-set meters keyed by source address**: per-key state is per host,
  contradicting the Meter's aggregate-over-the-whole-prefix semantics, and
  adds dynamic element lifecycle (timeouts, GC, set caps) to the packet
  path. Rejected.
- **Verdict map to per-prefix chains** (chosen): keeps one rule per ban rule
  and the O(log n) lookup; per-prefix state lives in chains reached only by
  matching traffic.

## Consequences

- Counters for limit stances live in the per-prefix chains: a rule-level
  counter can no longer sit after the map lookup, because a jump verdict
  ends the rule. The hit read-back aggregates chain counters per ban rule,
  keeping outcome totals unchanged.
- In-rate traffic falls off the chain end and returns to the caller's rule
  walk, preserving the known first-match limitation (tracked as an issue
  under the per-prefix meters feature).
- Meter state resets with every ruleset replacement (whole-table replace),
  as before.
