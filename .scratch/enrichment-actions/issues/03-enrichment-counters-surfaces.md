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

**Status:** ready-for-agent

- [ ] `counter: true` on an enrich action enables that (rule, enrichment) pair's counter; default `false`; a written non-boolean fails load; two enrich actions of one kind in one rule sum into one row
- [ ] `total`/`observed`/`enriched` count runs exactly under the worker pool - one event enriched twice counts twice - and are cumulative since daemon start
- [ ] The `/api/stats` snapshot's `enrichment` block lists rule, enrichment, `total`, `observed`, `enriched` in rules-file order, counter-enabled pairs only
- [ ] The Stats page's enrichment counters table renders through the shared counter-table mechanism with its explicit empty state and search-box narrowing
- [ ] Behavior is pinned at the assembled-daemon seam (log lines in, Stats out) and the panel pins; no test reaches into the counter registry
