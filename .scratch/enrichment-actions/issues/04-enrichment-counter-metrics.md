# 04: Enrichment counter metric families

**What to build:** A scraper reads the Enrichment counters from the
machine contract instead of polling a human surface: three counter
families carrying the labels `rule` and `enrichment` -
`bandage_enrichment_runs_total`, `bandage_enrichment_runs_observed_total`
and `bandage_enrichment_runs_enriched_total` - rendered after the rule
counter families with series in rules-file order and present from
daemon start with zeros so `rate()` and `increase()` work before the
first run. The metrics contract document gains the families' pinned
HELP text and their per-kind meanings (`observed` equals `total` for a
kind without learned state) in the same change, so the exposition and
the contract cannot drift.

**Blocked by:** 03

**Status:** resolved

- [x] The three counter families render in the exposition after the rule counter families, labels `rule` and `enrichment`, series in rules-file order, counter-enabled pairs only
- [x] Enabled series are present from daemon start with zeros
- [x] The metrics contract document states the families, their HELP text and the per-kind quantity meanings, and its reproduction test passes against the exposition
- [x] Adding the families renames and removes nothing in the existing contract

## Answer

Implemented on `ticket/ea-04-enrichment-metrics` (commits `9a7b106`,
`8c6a1eb`), merged into `integration/enrichment-actions` (`408e8dd`).
Three families render after the rule counter families, labels `rule`
and `enrichment`, counter-enabled pairs only:
`bandage_enrichment_runs_total`,
`bandage_enrichment_runs_observed_total`,
`bandage_enrichment_runs_enriched_total` - HELP "Enrichment runs for
the rule and enrichment since daemon start." / "Enrichment runs that
fed the enrichment's learned state since daemon start." / "Enrichment
runs that produced a non-default derived value since daemon start."
Series are in rules-file order via a dedicated non-sorting
`enrichmentSeries` - the deliberate asymmetry with the rule families'
rule-ID order is stated in docs/metrics.md's mirror paragraph and
pinned with a file-order-not-ID-order fixture (`zeta`, `alpha`) - and
enabled series are present from daemon start with zeros. The contract
doc gains the family rows, the pinned HELP text, and the per-kind
meanings (Filesize: observed = runs that fed the estimate from
Sampling-rule-eligible responses, enriched = runs that produced a
non-default derived value; a kind without learned state reports
observed equal to total); the metricsdoc reproduction test passes
against the exposition. Nothing renamed or removed (ADR 0014) - the
golden diff is purely additive. docs/rules.md documents the enrich
`counter` key and the `counter=true` plan line.

Context: spec `../spec.md` (user stories 33-36), ADR 0014,
`docs/metrics.md`.

## Comments
