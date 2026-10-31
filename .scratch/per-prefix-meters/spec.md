# Spec: per-prefix meters for limited ban rules

Status: ready-for-agent

## Problem Statement

When several Target prefixes carry the same limit Penalty (same action and
same stated rate), nftables enforcement installs one shared rate-limit state
across all of them: a single token bucket feeds every prefix in the group. An
operator who limits two client prefixes at 1000 packets/s expects each prefix
to get its stated 1000/s; instead one noisy prefix drains the shared budget
and the other is throttled - or dropped past the rate - despite doing nothing
wrong. The stated rate silently means "this rate per group of same-rate
prefixes" instead of "this rate per target", which contradicts the Penalty as
stated and the documentation.

## Solution

Each limited Target prefix owns its own **Meter**: the rate-limit state
carrying the Penalty's stated rate, aggregating over that one effective Target
prefix (the Report's Target as narrowed by exclusions) and never over its
neighbours. The nftables firewall driver installs limit ban rules as one
interval verdict map per ban rule and address family, whose elements jump to
one tiny chain per effective Target prefix - each chain holding that prefix's
counter, its limit expression (stated rate, derived burst), and the drop. The
packet path stays fast: one rule walk, one O(log n) map lookup, and a jump
into a two-expression chain only for matching traffic. Everything else -
drop, reject, limit-req-rate declines, enforcement scopes, the dry-run plan,
ruleset identity, hit reporting, atomic replacement, namespace targeting -
behaves exactly as today.

## User Stories

1. As an operator, I want each limited Target prefix to be metered independently, so that one abusive prefix can never consume another prefix's allowed rate.
2. As an operator, I want the stated rate of a limit Penalty to mean the rate per effective Target prefix, so that enforcement matches what the Penalty promises.
3. As a service client inside one limited prefix, I want my allowed rate to survive a noisy neighbour limited at the same rate, so that my traffic is throttled only by my own prefix's behaviour.
4. As an operator, I want each effective Target prefix - a Report's Target as narrowed by exclusions - to own its own Meter, so that exclusions carve the ban without silently merging budgets.
5. As an operator, I want each Meter to aggregate over its whole prefix and never per host inside it, so that a single client cannot sidestep a prefix-level limit.
6. As an operator, I want packet-rate and response-bandwidth limits each to keep their own Meter per prefix and dimension, so that two limit Penalties on one target never interfere.
7. As an operator, I want rate limits to keep their derived burst of one second's worth of the stated rate, so that legitimate micro-bursts survive enforcement.
8. As an operator, I want traffic past the rate dropped and traffic within the rate to fall through exactly as it does today, so that upgrading enforcement changes nothing except budget isolation.
9. As an operator, I want drop and reject ban rules to be installed unchanged, so that the metering redesign cannot regress the common stances.
10. As an operator, I want `limit-req-rate` to keep being declined with its stated reason, so that application-layer intent is never faked at the packet layer.
11. As an operator, I want enforcement scopes (transport protocols, ports, and addresses per direction) to constrain metered bans exactly as configured, so that a Meter only sees the protected service's traffic surface.
12. As an operator, I want IPv4 and IPv6 targets metered per prefix within their address family, so that a dual-compiled ban rule meters each family's prefixes independently.
13. As an operator, I want the dry-run plan to be unchanged, so that a metering upgrade produces no plan churn and my review workflow stays stable.
14. As an operator, I want ruleset identity to be unchanged, so that an upgrade with the same reports re-enforces exactly once and then skips as before.
15. As an operator, I want hit counters for every ban rule - including metered ones - reported as "since the last change" totals per ban rule, so that my log-reading workflow is unaffected.
16. As an operator, I want the owned table replaced atomically on every change, so that bans are never absent while new meters install.
17. As an operator, I want namespace-targeted enforcement points to keep working, so that container-adjacent deployments are unaffected.
18. As an operator, I want Meter state to reset when the ruleset changes, consistent with today's whole-table replacement, so that behaviour after an update is predictable.
19. As an operator, I want the operator documentation to describe limit enforcement in the glossary's terms - one Meter per effective Target prefix - so that the docs match what enforcement does.
20. As a maintainer, I want the per-prefix metering independence pinned by tests against a real kernel, so that silent budget sharing can never creep back in.
21. As a maintainer, I want the emitted kernel shape for limit stances pinned at the driver's fake-kernel seam, so that encoding regressions are caught without root.
22. As a maintainer, I want the change confined to the nftables firewall driver, so that the compiler, the ban-rule contract, and every other consumer of the firewall seam are untouched.

## Implementation Decisions

- The change is confined to the nftables firewall driver. The domain
  compiler, the ban-rule shape, the plan output, the ruleset hash, and the
  outcome contract are all unchanged.
- For the limit stances (packet rate, response bandwidth) the driver replaces
  the plain target interval set plus inline limit expression with one
  interval verdict map per (ban rule, address family); each map element
  jumps to one private chain per effective Target prefix.
- Each such chain holds exactly three expressions, in order: a counter, a
  limit expression with the stated rate, the derived burst, and inverted
  ("over") semantics, then a drop verdict. The chain's limit state is the
  Meter - one per effective Target prefix per dimension.
- In-rate traffic does not match the chain's limit expression, falls off the
  chain's end, and returns to the calling rule walk - preserving today's
  fall-through semantics exactly, including the documented first-match
  limitation (see Out of Scope).
- Non-limit stances (drop, reject) keep their existing interval sets and
  rule-level counters.
- Counters for limit stances move into the per-prefix chains: a rule-level
  counter can no longer sit after the map lookup, because a jump verdict ends
  the rule. The hit read-back aggregates chain counters per ban rule exactly
  as before, so "since the last change" totals per ban rule are unchanged.
- Naming: the verdict map keeps the ban rule's existing per-rule, per-family
  name; each chain is named after its map element slot (map name plus slot
  suffix), so the hit read-back maps a chain to its ban rule by stripping the
  trailing slot segment. The existing ban-rule-index extraction from lookup
  expressions is untouched.
- One map per (ban rule, address family): the equal-length prefixes inside
  one map are disjoint, and most-specific-first ordering across rules is
  preserved. Verdict maps introduce no new kernel floor.
- Meter unit: one Meter per effective Target prefix. When exclusions split
  one Report's Target into several effective prefixes, each piece carries its
  own Meter and its own share of the stated rate (the total budget grows with
  the number of pieces). Reports that compile to the same effective prefix,
  stance, and rate merge into one ban rule and share one Meter - they are
  indistinguishable at enforcement.
- Meter state resets whenever the ruleset is replaced (the owned table is
  replaced wholesale) - unchanged behavior, noted so it is not mistaken for a
  regression.
- Terminology: the rate-limit state is called a **Meter** (now in the
  glossary); "bucket" remains reserved for the detection-side tracker
  vocabulary and stays on its avoid-list.

## Testing Decisions

- A good test asserts external behavior: what the firewall driver installs
  and what packets experience at its enforcement surface. Netlink attribute
  encodings are pinned only where the encoding itself is the contract (the
  existing shared-encoding-shape assertions).
- Two seams, both existing - no new seams. The highest seam is the firewall
  driver's plan/apply/reset port exercised against the fake in-kernel
  stand-in; the second is the root-gated real-kernel suites (install,
  read-back, reset; scratch namespace).
- At the fake-kernel seam: assert the shape installed for limit stances -
  one verdict map per ban rule and family, one chain per effective Target
  prefix with expressions in counter/limit/drop order - and that drop/reject
  shapes are unchanged.
- At the real-kernel seam: the independence pin - install two same-rate
  limited prefixes in a scratch namespace, saturate one Meter with traffic,
  and assert the sibling prefix's traffic is unaffected while the saturated
  prefix is throttled past its rate; also assert in-rate traffic still falls
  through. This covers new ground: today's namespace tests install and read
  back without driving packets.
- Modules tested: the nftables firewall driver only. The compiler is
  untouched and the whole existing suite must keep passing unchanged.
- Prior art: the fake kernel's shared encoding-shape assertions, the
  root-gated install/read-back/reset test, and the scratch-namespace install
  test.

## Out of Scope

- The documented first-match meter limitation: in-rate traffic of a
  more-specific Meter falls through to a broader ban rule below and is still
  dropped there. Fixing it is a semantic decision of its own - filed as
  issue 01.
- Surfacing per-prefix Meter hits in outcome logs: the chain counters hold
  the data, but only per-ban-rule totals are reported - filed as issue 02.
- Compiler and merge-policy changes, ban-rule shape changes, and any
  enrichment of the dry-run plan or ruleset hash.
- Dynamic-set ("dynset" / nft meter) alternatives to per-prefix chains.
- Any change to limit-req-rate handling: it stays declined with its reason.
- Other firewall drivers.

## Further Notes

- Rejected alternative - one kernel rule per limited prefix: trivially gives
  per-prefix limit state but makes every packet walk the rule chain linearly
  (the worst case is benign traffic, which matches nothing) and discards the
  O(log n) interval lookup. Fine at tens of prefixes, harmful at hundreds or
  thousands.
- Rejected alternative - dynamic meters keyed by source address: per-key
  state is per host, contradicting the Meter's aggregate-over-the-whole-
  prefix semantics, and introduces dynamic element lifecycle (timeouts, GC,
  set size caps) on the packet path.
- Nested ban rules are unchanged: a packet matching a more-specific Meter
  and a broader rule below can still consume both (fall-through) - exactly as
  today.
- The decision and its alternatives are recorded in ADR 0020.
