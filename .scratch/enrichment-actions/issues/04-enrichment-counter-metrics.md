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

**Status:** ready-for-agent

- [ ] The three counter families render in the exposition after the rule counter families, labels `rule` and `enrichment`, series in rules-file order, counter-enabled pairs only
- [ ] Enabled series are present from daemon start with zeros
- [ ] The metrics contract document states the families, their HELP text and the per-kind quantity meanings, and its reproduction test passes against the exposition
- [ ] Adding the families renames and removes nothing in the existing contract
