# 03: Enrichment counters on the operator surfaces

**What to build:** A rules author adds `counter: true` to an enrich
action and an operator sees that (rule, enrichment) pair's Enrichment
counter wherever they read Rule counters: the `/api/stats` snapshot
gains an `enrichment` block and the Web Panel's Stats page a matching
table - rows of rule, enrichment, `total`, `observed`, `enriched`, in
rules-file order, counter-enabled pairs only. `total` counts the
enrichment's runs (not events), `observed` the runs that fed its
learned state, `enriched` the runs that produced a non-default derived
value; counts are exact under the worker pool, cumulative since daemon
start and reset on restart like every other Stats number. The table
adopts the prefactored counter-table mechanism: explicit empty state
and shared search-box narrowing. The `counter` key joins the enrich
block's strict validation - optional, default `false`, strictly boolean
when written.

**Blocked by:** 01, 02

**Status:** resolved

- [x] `counter: true` on an enrich action enables that (rule, enrichment) pair's counter; default `false`; a written non-boolean fails load; two enrich actions of one kind in one rule sum into one row
- [x] `total`/`observed`/`enriched` count runs exactly under the worker pool - one event enriched twice counts twice - and are cumulative since daemon start
- [x] The `/api/stats` snapshot's `enrichment` block lists rule, enrichment, `total`, `observed`, `enriched` in rules-file order, counter-enabled pairs only
- [x] The Stats page's enrichment counters table renders through the shared counter-table mechanism with its explicit empty state and search-box narrowing
- [x] Behavior is pinned at the assembled-daemon seam (log lines in, Stats out) and the panel pins; no test reaches into the counter registry

## Answer

Implemented on `ticket/ea-03-enrichment-counters` (commit `dafca41`),
merged into `integration/enrichment-actions` (`f2dbbde`). `counter`
joins the enrich block's strict validation (`parseEnrichCounter`,
mirroring the rule-level `parseCounter`: optional, default `false`,
strictly boolean when written).
`app/enrichmentcounters.go` is `rulecounters.go`'s twin behind the
`evaluation.EnrichmentCounters` port: one row per (rule, kind) pair in
rules-file order of pairs (first occurrence across `rule.ActionLists()`
- ladder rungs included), the pair enabled when any of the rule's
enrich actions of that kind carries `counter: true`, every run of that
kind at the rule summed into the row, atomics exact under the worker
pool, cumulative since daemon start. `/api/stats` gains the
`enrichment` block (rows of `rule`, `enrichment`, `total`, `observed`,
`enriched`, counter-enabled pairs only, `[]` when none); the Stats page
gains the matching table through the shared counter-table mechanism
(ticket 01) with the explicit empty state
`No enrichment counters configured.` and shared search-box narrowing.
Pins at the assembled-daemon seam (quantity movements,
double-traversal counts twice, same-kind-per-rule summing and any-flag
registration, 4-worker exactness), the panel pins, and loader pins for
non-boolean `counter`; nothing reaches into the registry.

Context: spec `../spec.md` (Implementation Decisions, user stories
18-29), GLOSSARY (Enrichment counter).

## Comments
2026-11-04, post-merge hotfix: the registry adoption shipped a
load-time ReferenceError - `COUNTER_TABLES`'s initializer named
`ENRICHMENT_COUNTERS_TABLE` before its `const` (temporal dead zone),
 so common.js died at load and the whole panel went inert (browser
console: "can't access lexical declaration ... before
initialization"). The served-bytes pins and `node --check` cannot see
load order. Fixed by declaring every config above the registry;
`TestCounterTableConfigsAreDeclaredBeforeTheRegistry` now pins the
order for the next adopted table.
