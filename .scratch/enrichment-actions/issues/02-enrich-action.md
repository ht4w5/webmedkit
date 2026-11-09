# 02: Enrich action - demand-driven enrichment

**What to build:** A rules author writes an `enrich` action in a rule's
action list - `actions: [{enrich: {type: filesize}}]`, one enrichment
per step, order explicit in the list - and the named enrichment runs
where the rule matches: after that rule's tracker update, feeding the
derived fields of every subsequent rule. The documented composition -
a tracker-less enrich rule ahead of the consuming replay rule - turns
blind replay warm-up into live volume tracking. The enrichments are
global and static: one instance per kind, shared by every enrich action
naming it. The old world is gone in the same change: the pre-evaluation
enrichment path and the top-level `enrichment` block disappear, and an
old-shape rules file fails load with a pointed error naming the
migration. An unknown `type` or an unknown key in the enrich block
fails load naming it. The `bandage -t` plan shows enrich actions inline
in their rule's action list and prints no enrichment filter line.

**Blocked by:** None

**Status:** resolved

- [x] An enrich action runs its named enrichment where every action runs (on Match, after its own rule's tracker update) and is non-terminal; derived fields feed subsequent rules only
- [x] The tracker-less-enrich-rule-ahead-of-a-replay-rule composition works end to end at the assembled-daemon seam: reports appear where the tracker was blind before
- [x] Demand-driven learning holds: an eligible response contributes an observation only when its event reaches an enrich action; untouched paths create no enrichment state
- [x] One event traversing two enrich actions runs and observes twice - counted and documented as the rules author's responsibility, with no memoization
- [x] The rules file's top-level `enrichment` block fails load with a pointed migration error; unknown `type` and unknown keys in the enrich block fail load naming them; an absent enrich block means no enrichment runs
- [x] The plan prints enrich actions in their action list and no enrichment filter line
- [x] Rules documentation covers the action, the ordering caveat and the tracker-less composition; the behavior is pinned by end-to-end tests, not internals
- [x] The full suite passes race-enabled with the pre-evaluation path removed

## Answer

Implemented on `ticket/ea-02-enrich-action` (commit `137f84a`), merged
into `integration/enrichment-actions` (`da3f50d`). `enrich: {type:
...}` joins the action model (non-terminal, on Match after its own
rule's tracker update, rule and rung action lists alike); the
evaluation service invokes an `Enrichments` seam by kind name with the
event - returning (observed, enriched, err) - and counts runs through
the `EnrichmentCounters` port (evaluator-owned counting points;
daemon-owned instances, one global instance per kind via
`enrichmentBuilders`). `resetDerived` moves to `evaluation.Evaluate`
entry (ADR 0019 gains an "Amended by ADR 0028" line). The
pre-evaluation path and the top-level `enrichment` block die wholly:
any presence of the block fails load naming the migration; unknown
`type` and unknown keys fail load naming them. The plan shows enrich
actions inline and prints no enrichment filter line. Demand-driven
learning, the tracker-less composition and the no-memoization
double-run are pinned at the assembled-daemon seam
(`enrichaction_test.go` replaces `enrichgate_test.go`); fixtures that
relied on implicit enrichment carry explicit tracker-less enrich
rules. docs/rules.md gains the enrich section (ordering caveat incl.
terminal starvation, tracker-less composition, no-memoization note).
Review fixes: pointed load failures pinned at the daemon seam
(`0cbb271`), stop-skew named (`d62589f`), GLOSSARY Action vocabulary
(`8a83344`). Run-counting exactness pins landed with ticket 03
(`dafca41`).

Context: spec `../spec.md` (Implementation/Testing Decisions), ADR
0028, GLOSSARY (Sampling rule).

## Comments
Review follow-up (dynamic-reports review): the pointed migration check
for the top-level `enrichment` block is gone (no checks for removed
rule syntax); the strict decoder refuses the unknown key, so the old
shape still fails load. Checklist line 2's "pointed migration error"
records the original delivery, amended at review.
