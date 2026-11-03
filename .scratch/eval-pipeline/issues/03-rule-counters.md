# 03: Rule counters on the operator surfaces

**What to build:** A rules author sets `counter: true` on a rule and
the operator sees its Rule counter move on the surfaces they already
read: the `/api/stats` snapshot gains a `rules` block (`id`, `total`,
`matched`, rules-file order, counter-enabled rules only) and the Web
Panel's Stats page gains a rule counters section after the tracker
cache group - a rule/total/matched table narrowed by the shared search
box, with an explicit "no rule counters configured" empty state. The
counts are honest: `total` is every event the rule is evaluated
against (filter rejects and replay warm-up skips included, one visit
per dispatch of a shared chain), `matched` is exactly the rule's
Matches (one per matching Event however many ladder rungs fired),
cumulative since daemon start, reset on restart, exact under the worker
pool. `counter` defaults to false - upgrading adds no noise until
asked for.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A rule-level `counter: true/false` key (default false) is
  accepted by the rules format; strict decode rejects malformed values.
- [x] `total` increments once per rule visit: filter rejects and
  replay warm-up skips included; a chain dispatched more than once for
  one event counts one visit per dispatch (test pins it).
- [x] `matched` increments once per Match; a ladder counts one per
  matching Event regardless of rungs fired.
- [x] Counters are cumulative since daemon start, in-memory only,
  keyed by rule ID in rules-file order, enabled rules only, and exact
  under the worker pool.
- [x] `/api/stats` carries the `rules` block in rules-file order with
  `{id, total, matched}` per counter-enabled rule.
- [x] The Stats page shows the rule counters table, narrows it by the
  search box, and shows the explicit empty state when nothing is
  enabled.
- [x] Tests pin the counting semantics at the daemon end-to-end seam,
  the key handling at the rules-loader seam, and the served surfaces
  at the panel's served-surface seam.

## Answer

Implemented on `ticket/ep-03-rule-counters` (commits `1b416c6`,
`df4cf3a`, `147b0d0`, `2c2da9d`), merged into `integration/eval-pipeline`
(`6819f03`). Rule-level `counter` key (default false) with key-naming
validation; in-memory `app.ruleCounters` registry (atomic counters,
rules-file order, counter-enabled rules only, cumulative since start);
evaluator counting via a narrow `evaluation.Counters` port
(`Deps.Counters`, explicit `NopCounters` after the review fix pass):
`total` one Visit per rule-loop iteration - filter rejects and replay
warm-up skips included, one visit per dispatch of a shared chain -
and `matched` one Match per Match, a ladder counting once per matching
Event. `/api/stats` gains the `rules` block (`[{id,total,matched}]`,
rules-file order, `[]` when none); the Web Panel Stats page gains the
rule counters table after the tracker cache group, narrowed by the
shared search box, with the explicit `No rule counters configured.`
empty state. Review fix pass (`f8f84c2`): the block dispatches through
`STATS_GROUPS` and the table shape is single-sourced from the thead.
Pins at the daemon e2e seam (counting semantics incl. one-visit-per-
dispatch and worker-pool exactness), the loader seam (key handling)
and the panel served-surface seam. Vocabulary: glossary **Rule
counter**.

Context: spec `../spec.md` (Implementation/Testing Decisions),
`GLOSSARY.md`.

## Comments
