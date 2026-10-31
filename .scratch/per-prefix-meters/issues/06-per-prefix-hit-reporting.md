# 06: Surface per-prefix Meter hits in outcome logs

**What to build:** With per-prefix meters, each effective Target prefix's
chain carries its own counter, but outcome logs keep reporting only
"since the last change" totals per ban rule. Surface the per-prefix
breakdown - e.g. extend the install log line (or the outcome the driver
returns) with per-prefix packet/byte counts - so an operator can see *which*
limited prefix is consuming its budget. Per-ban-rule totals must remain as
they are today.

**Blocked by:** 04: Per-prefix Meters for limit-bandwidth - both stances'
chain counters and the chain-aware hit read-back land there.

**Status:** resolved

- [x] Decide the output shape (log fields vs. outcome structure) - this
      extends a contract the repo treats as curated, so the shape wants a
      human call.
- [x] Extend the driver's hit read-back to carry per-prefix counts (already
      parsed from chain names) up to the outcome.
- [x] Document the new fields wherever the install-log format is described.

## Answer

Maintainer decision (2026-06): **a typed outcome structure, rendered
into logs.** `firewall.Hit` gains `Prefixes []PrefixHit` (`Target`,
`Packets`, `Bytes`), where `Target` is the effective Target prefix as a
CIDR string from the replaced ruleset - the same text the dry-run plan
shows, never the internal chain slot name. Per-rule totals
(`Hit.Rule/Packets/Bytes`) stay byte-identical in meaning and shape;
only nonzero entries report (a Hit appears only when its rule counted
something, its Prefixes only nonzero prefix counters); drop and reject
ban rules carry no breakdown. The existing `ban rule hit in the
replaced ruleset` log line stays byte-identical; after their rule's
total line comes one `meter hit in the replaced ruleset` line per
prefix entry, fields point, rule, prefix, packets, bytes.

Implemented on `ticket/ppm-06-per-prefix-hit-reporting`, to be merged
into `feat/per-prefix-meters`. The chain-aware hit read-back already
parses each Meter chain's `<map>.<slot>` (ticket 01); it now maps
(family, slot) onto the effective Target prefix through the replaced
ruleset - the previous apply's input rules, the ruleset `Hit.Rule`
already indexes, now kept in the driver's skip state - and carries the
per-prefix counts up through `Outcome`. Entries order like the rule's
effective targets (family, then slot in the family's target list). The
curated `firewall.Driver` contract's doc comments cover the sanctioned
extension (what: per-prefix Meter readings behind a metered rule's
total; why: ADR 0020's chains hold one counter per prefix, so the log
can say which limited prefix is consuming its budget).

Coverage: `fakekernel_test.go`
(`TestApplyBreaksDownMeteredHitsPerEffectiveTargetPrefix`: per-prefix
entries with correct CIDR strings and counts in effective-target
order, drop/reject yield none, totals unchanged, unnamed slots keep
totals only), the root-gated read-back
(`TestKernelAttributesOwnedChainCountersByChainName` seeds one mappable
Meter reading and pins its breakdown through `Outcome`; the drop
rule's slot name and an out-of-range slot pin the totals-only cases),
and all three traffic pins (the per-prefix entries of every metered
rule's hit, through the operator path). Engine logging:
`TestUpdateCycleLogsThePerPrefixMeterHits` pins one line per prefix
after its rule's total line with fields point, rule, prefix, packets,
bytes. Docs: `docs/splint.md`'s cycle-log example and prose carry the
new line and its fields (the example's counting rule is now a
`limit-pkt-rate` Report so its breakdown can show).

Context: `../../splint/internal/firewall/driver.go` (`Hit.Prefixes`,
`PrefixHit`), `../../splint/internal/firewall/nft/nft.go`
(`ruleHits`, `meterTarget`, `meterChainSlot`),
`../../splint/internal/app/updater.go`, `docs/splint.md`.

Notes:
- The breakdown maps through the last install's input rules
  (`d.prevRules`). After a restart the first read-back reports totals
  only: indexes and totals survive (set and chain names bake in the
  index), but a fresh instance cannot name the replaced ruleset's
  prefixes. Every pre-existing assertion passed untouched - totals are
  byte-identical.
- The mapping is stance-gated: only a metered rule owns Meters, so a
  slot name under a drop or reject rule keeps its total without a
  breakdown (pinned at both seams).
- The root-gated suites were exercised for real in an unprivileged
  user+net namespace (`unshare --user --map-root-user --net`), all
  green; `make test` green. The slot-to-prefix pin was validated red
  against a throwaway reversed-slot mutation.

## Comments

Filed from the per-prefix meters design interview (2026-06); accepted as a
follow-up when the counters moved into the meter chains. Renumbered from 02
when implementation tickets landed ahead of it.
