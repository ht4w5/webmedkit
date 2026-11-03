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

**Status:** ready-for-agent

- [ ] The exposition renders `bandage_rule_events_total{rule}` and
  `bandage_rule_events_matched_total{rule}` after the tracker-cache
  families, series in rule-ID order, zeros from daemon start.
- [ ] The metrics contract document gains the family rows and HELP
  text, the widened labels paragraph, and the
  cardinality-per-configuration amendment.
- [ ] The contract's pinned example is regenerated and the byte-pin
  test is green; the contract and exposition cannot drift.
- [ ] The periodic stats log record is unchanged (asserted by test or
  review note).

## Comments
