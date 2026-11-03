# 04: Rule counter metric families

**What to build:** A scraper reads the Rule counters from the machine
contract: two new counter families in the hand-rendered exposition,
`bandage_rule_events_total{rule}` (events evaluated against the rule
since daemon start) and `bandage_rule_events_matched_total{rule}`
(events that matched the rule since daemon start), rendered after the
tracker-cache families with series in rule-ID order. Enabled rules
emit from daemon start with zeros, so `rate()` and `increase()` work
before the first match. The metrics contract document states the
families and their HELP text, widens its labels paragraph to three
label-carrying families, and amends the cardinality promise to
fixed-per-configuration - the rule families' series count is the number
of counters enabled in the rules file, never a function of traffic -
with its byte-pinned example regenerated. The periodic stats log line
stays unchanged by decision.

**Blocked by:** 03 (Rule counters on the operator surfaces)

**Status:** resolved

- [x] The exposition renders `bandage_rule_events_total{rule}` and
  `bandage_rule_events_matched_total{rule}` after the tracker-cache
  families, series in rule-ID order, zeros from daemon start.
- [x] The metrics contract document gains the family rows and HELP
  text, the widened labels paragraph, and the
  cardinality-per-configuration amendment.
- [x] The contract's pinned example is regenerated and the byte-pin
  test is green; the contract and exposition cannot drift.
- [x] The periodic stats log record is unchanged (asserted by test or
  review note).

## Answer

Implemented on `ticket/ep-04-counter-metrics` (commits `8f17049`,
`c24c0b0`), merged into `integration/eval-pipeline` (`05aeff0`).
`/metrics` gains `bandage_rule_events_total{rule}` and
`bandage_rule_events_matched_total{rule}` after the tracker-cache
families - HELP "Events evaluated against the rule since daemon
start." / "Events that matched the rule since daemon start." - one
series per counter-enabled rule, present from daemon start with
zeros. The `metricFamily` table gained per-family `series`
(multi-series; `scalar` helper behind one-value families from the
review fix pass `050ddb7`). docs/metrics.md: family rows, labels
paragraph widened to three label-carrying families, cardinality
amended to fixed-per-configuration (series count = counters enabled in
the rules file, never a function of traffic), byte-pinned example
regenerated (fixture-golden-doc-fence triangle moved together).
Adjudication: series render sorted by rule ID (the spec's "rule-ID
order") while the `rules` block keeps rules-file order - the snapshot
mirror is fields and values, not ordering. The periodic stats log
record is unchanged, asserted by `TestE2EStatsLogRecordKeepsItsFieldSet`.
Per ADR 0014's add-series policy, no ADR change.

Context: spec `../spec.md`, `docs/metrics.md`, ADR `docs/adr/0014`.

## Comments
