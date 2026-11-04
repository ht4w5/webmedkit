# Enrichment actions: demand-driven enrichment and Enrichment counters - spec

Status: resolved

## Problem Statement

The Filesize estimate learns from every event the enrichment path
touches, whether or not any rule ever consumes the result. A burst of
first-seen paths - a crawl across rarely used repository files - fills
the enrichment cache with records nothing reads, and because file-size
records share the tracker store's entry cap and per-shard LRU budget,
that churn evicts live tracker state: accumulated volumes silently
restart while the estimates sit unused.

The enrichment gate cannot help precisely: `enrichment.filter` is a
second scoping predicate, hand-copied from the rule filters that
actually define which traffic matters, drifting from them the moment
one side is edited - and as a static predicate it can never see
evaluation state.

And enrichment is invisible: an operator asking "is my replay estimate
warming, and at what cost in cache traffic?" has no number to look at.
The Stats snapshot counts lines and tracker-cache traffic, Rule
counters cover rules - nothing covers enrichment.

## Solution

Enrichment becomes an **Action** in the evaluation pipeline: the rules
author writes an `enrich` action where the derived fields are needed,
and enrichment runs there and nowhere else. The rules file becomes the
single source of truth for which events get enriched - the enrich
action's rule filter is the demand domain, so scoping cannot drift from
the rules it serves. Traffic no rule asks about observes nothing and
creates nothing: bursts of unused paths cost no cache entries and
cannot evict tracker records.

The enrichments themselves are global and static: one instance per
enrichment kind, shared by every enrich action naming it (`type:
filesize`), with no per-action instance or configuration. Each enrich
action carries its own `counter: true`, giving that (rule, enrichment)
pair an **Enrichment counter** - its `total` runs, the subset that fed
the estimate (`observed`) and the subset it successfully enriched
(`enriched`) - on every surface the Rule counters already reach.

## User Stories

1. As a rules author, I want enrichment to run only where my rules ask
   for it via an `enrich` action, so that "which events get enriched"
   is decided in the same file as everything else my rules decide.
2. As a rules author, I want my rule's filter to be the enrichment's
   demand domain, so that the scoping cannot drift from the rules it
   serves the way a duplicated `enrichment.filter` did.
3. As a rules author, I want the action written as `enrich: {type:
   filesize, counter: true}`, so that it reads like every other action
   in my action list.
4. As a rules author, I want one enrichment per `enrich` step, so that
   execution order between two enrichments is explicit in my action
   list like every other ordering.
5. As a rules author, I want the enrich action to run where every
   action runs - when its rule matches - so that Actions keep one
   uniform meaning.
6. As a rules author, I want the enrich action to feed the rules after
   it and never its own rule's tracker update, so that its position in
   the file is meaningful and honest.
7. As a rules author, I want the documented composition for feeding a
   replay tracker - a tracker-less enrich rule ahead of the consuming
   rule - so that I set up demand-driven enrichment without surprises.
8. As a rules author, I want every enrich action naming `filesize` to
   share one global enrichment instance, so that the estimate learns
   cumulatively no matter which rule asked.
9. As a rules author, I want no per-action enrichment configuration,
   so that one enrichment kind means one thing across my file.
10. As a rules author, I want `counter: true` on my enrich action to
    opt that (rule, enrichment) pair into counting, so that I watch the
    enrichments I care about and leave the rest quiet.
11. As a rules author, I want `counter` to default to false and to
    fail load when written as anything but `true` or `false`, so that a
    typo is a load error, never a silent default.
12. As a rules author, I want an enrich action naming an unknown
    enrichment type to fail load with an error naming the type, so that
    a typo can never silently enrich nothing.
13. As a rules author, I want unknown keys inside the `enrich` block
    to fail load, so that a misspelled key is caught before runtime.
14. As a rules author, I want the old top-level `enrichment` block to
    fail load with a pointed error naming the migration to enrich
    actions, so that I fix my rules file once, loudly.
15. As a rules author, I want disabling an enrichment to mean removing
    its enrich actions, so that my file always describes exactly what
    runs - no hidden runtime switch.
16. As a rules author, I want the `bandage -t` plan to show my enrich
    actions in the action list like every other action, so that I can
    audit demand-driven enrichment before shipping the file.
17. As a rules author, I want enrichment to remain non-terminal like
    every action other than `goto` and `stop`, so that my chain
    continues after enriching.
18. As a rules author, I want two enrich actions of the same type in
    one rule to sum into one counter row, so that the counts match "my
    rule's enrichment of this kind", not plumbing.
19. As a rules author, I accept that nothing memoizes enrichment per
    event: if my rules enrich one event twice, it runs twice and
    observes twice - preventing that is mine to do.
20. As an operator, I want an Enrichment counter's `total` to count
    the enrichment's runs, so that the number is honest about what the
    daemon did.
21. As an operator, I want `observed` to count the runs that fed the
    estimate, so that I see the cache-growth pressure my rules create.
22. As an operator, I want `enriched` to count the runs that produced
    a non-default derived value, so that I can see whether the estimate
    is warm where I rely on it.
23. As an operator, I want the three quantities on every Enrichment
    counter even when a kind learns nothing (`observed` equal to
    `total`), so that my tables and queries keep one shape.
24. As an operator, I want Enrichment counters identified per (rule,
    enrichment), so that a number points at the exact decision that
    created it.
25. As an operator, I want the counts cumulative since daemon start
    and reset on restart like every other Stats number, so that I read
    them the way I read the pipeline counters.
26. As an operator, I want the counts exact under the worker pool, so
    that a busy daemon's numbers are trustworthy.
27. As an operator, I want an `enrichment` block in the `/api/stats`
    snapshot listing each enabled counter's rule, enrichment, `total`,
    `observed` and `enriched` in my rules file's order, so that the
    snapshot reads top to bottom like my config.
28. As an operator, I want an Enrichment counters section on the Web
    Panel's Stats page beside the rule counters, so that the numbers
    are on screen where I already look.
29. As an operator, I want an explicit "no enrichment counters
    configured" empty state in that section, so that silence is never
    mistaken for a broken panel.
30. As an operator, I want the panel's search box to narrow the
    enrichment counter rows like every other row, so that I find one
    rule in a long file.
31. As an operator, I want an eligible response to feed the estimate
    only when its event reaches an enrich action, so that the estimate's
    coverage matches my rules' scope and nothing more.
32. As an operator, I want the warm-up blindness and stop-skew
    consequences of demand-driven learning documented with the Sampling
    rule, so that a slow-warming path is never a mystery.
33. As a scraper, I want counter families
    `bandage_enrichment_runs_total{rule,enrichment}`,
    `bandage_enrichment_runs_observed_total{rule,enrichment}` and
    `bandage_enrichment_runs_enriched_total{rule,enrichment}`, so that
    I alert on enrichment load and estimate health from the machine
    contract.
34. As a scraper, I want the enrichment families rendered after the
    rule counter families with series in rules-file order, so that the
    exposition's deterministic rendering holds and diffs stay readable.
35. As a scraper, I want counter-enabled series present from daemon
    start with zeros, so that `rate()` and `increase()` work before the
    first run.
36. As a scraper, I want the metrics contract document to pin the
    families' HELP text and meanings per enrichment kind, so that the
    machine contract and the exposition cannot drift.
37. As a rules author, I want the rules documentation to explain the
    enrich action, its ordering caveat and the tracker-less
    composition, so that demand-driven enrichment is learnable from the
    docs alone.

## Implementation Decisions

- A new action joins the action model: the enrich action names exactly
  one enrichment kind by `type` and optionally enables its counter. It
  is non-terminal (only goto and stop are terminal, ADR 0027) and runs
  where every action runs - when its rule matches, after that rule's
  tracker update (ADR 0028). Derived fields therefore feed subsequent
  rules only; the documented composition is a tracker-less enrich rule
  ahead of the consuming rule.
- The enrichments are global and static: one instance per kind, built
  at daemon assembly and shared by every enrich action naming that
  kind. No per-action instance, no per-action enrichment settings.
- The evaluation service takes the enrichments through a collaborator
  seam it invokes by kind name with the event - the same pattern as the
  Rule counter seam: the daemon owns the instances and the counter
  registry; the evaluator owns the counting points.
- Nothing memoizes enrichment per event. One event may run the same
  enrichment twice (two enrich actions traversed, or a chain dispatched
  twice for one event), observing and counting twice. Counters
  therefore count runs, not events. Preventing double enrichment is the
  rules author's responsibility.
- The Enrichment counter is identified per (rule, enrichment kind).
  Quantities: `total` (runs), `observed` (runs that fed the kind's
  learned state), `enriched` (runs that produced a non-default derived
  value). The three-quantity shape is fixed for every kind; meanings
  are defined per kind - a kind without learned state reports `observed`
  equal to `total`, never an omitted column. Counting is exact under the
  worker pool and cumulative since daemon start.
- The single derived-field reset stays where ADR 0019 put it: once at
  evaluation entry, before any rule runs.
- Demand-driven learning: an observation happens only when an
  enrichment runs for the event (Sampling rule, GLOSSARY amended).
  Terminal actions upstream of the enrich rule starve the estimate on
  that path - accepted and documented (ADR 0028).
- Rules format: the top-level `enrichment` block is removed. Its
  presence fails load with a pointed error naming the migration to
  enrich actions; the rules file's `version: 0` stays. Inside the
  enrich block, `type` is required and must name a known kind; `counter`
  is optional (default `false`) and strictly boolean when written;
  unknown keys and unknown kinds fail load with errors naming them.
- Operator surfaces, mirroring the Rule counters: the `/api/stats`
  snapshot gains an `enrichment` block (rows of rule, enrichment,
  `total`, `observed`, `enriched`, rules-file order, counter-enabled
  only); the Web Panel Stats page gains a matching table with an
  explicit empty state and shared search-box narrowing; the Metrics
  contract gains three counter families
  (`bandage_enrichment_runs_total`,
  `bandage_enrichment_runs_observed_total`,
  `bandage_enrichment_runs_enriched_total`, labels `rule` and
  `enrichment`), rendered after the rule counter families, series from
  daemon start with zeros, series in rules-file order. The metrics
  contract document is updated in the same change as the exposition
  (ADR 0014: series may be added, never renamed).
- The `bandage -t` plan stops printing an enrichment filter line and
  shows enrich actions inline in their rule's action list.
- The Filesize enrichment's tuning (EWMA period, minimum samples) stays
  a build-level constant; making it configurable is out of scope.
- Record-keeping already landed: GLOSSARY.md carries the Enrichment
  counter term and the amended Sampling rule; ADR 0028 records the
  demand-driven decision and its rejected alternatives.

## Testing Decisions

- A good test asserts external behavior only: raw log lines in,
  operator-visible outcomes out - reports, the `/api/stats` snapshot,
  the metrics exposition, plan text, load errors. No test reaches into
  counter registries or enrichment internals.
- Primary seam: the assembled-daemon end-to-end seam - rules YAML as
  the input surface, a started test daemon fed synthetic access-log
  lines, outcomes read from the operator surfaces. Prior art: the
  enrichment-gate end-to-end suite and the rules end-to-end tests in
  the app package (send lines, wait for processing, read stats).
  Behavioral coverage lands here: demand-driven learning (an eligible
  response observes only when its event reaches an enrich action),
  enrichment feeding subsequent rules only, run-based counting under
  double traversal, the `total`/`observed`/`enriched` movements, and
  every pointed load failure (old `enrichment` block, unknown `type`,
  bad `counter`, unknown keys).
- Secondary seam: the surface pin tests - the Web Panel Stats page pins
  (table slot, empty state, search narrowing, single-sourced shape) and
  the metrics contract document reproduction test. Prior art: the rule
  counters' panel pins and the metrics doc test. These extend the
  existing pins rather than adding new harnesses.
- Fallback only: the evaluation package's stub-collaborator harness,
  for ordering edges the end-to-end seam expresses awkwardly (two
  enrich actions of one kind in one rule summing into one row).
- The suite runs race-enabled; counter exactness under the worker pool
  is asserted at the end-to-end seam on a busy daemon.

## Out of Scope

- Isolating the Filesize records' entry cap and eviction from the
  tracker store's LRU budget - filed as follow-up work in the issue
  tracker (a legitimate-but-huge path set can still compete with
  tracker records; demand-driven enrichment only stops junk entries).
- Making the Filesize tuning (EWMA period, minimum samples)
  configurable.
- A runtime kill switch for enrichments: `enabled` is deliberately not
  part of the shape.
- Memoization or at-most-once semantics for enrichment per event.
- New enrichment kinds beyond Filesize; the shape merely admits them.
- Any change to the report document contract, reason namespacing,
  tracker semantics, or the Web Panel beyond the Stats page table.

## Further Notes

- The decision and its rejected alternatives are recorded as ADR 0028
  (enrichment as rule actions); the vocabulary is in GLOSSARY.md
  (Enrichment counter; amended Sampling rule).
- The old gate's semantics die wholly: there is no replacement
  predicate, and the rules-file order of enrich actions relative to the
  rules that consume derived fields becomes meaningful - the docs must
  carry the ordering caveat prominently.
- `observed` equal to `total` for stateless kinds is a documented
  contract; only Filesize ships now, so the case is exercised through
  the contract text and the fixed table shape until a second kind
  exists.
