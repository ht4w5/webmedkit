# Spec: main-chain entry evaluation

Status: ready-for-agent

## Problem Statement

Rule authors think of a rules document like the firewall rule sets they
already know: one pipeline entered at a single place, with helper chains
dispatched on demand. bandage instead runs **every** chain over every
event in file order, so the rules don't behave the way they read: a
chain meant as a `goto`/`jump` target also sees events nobody dispatched
to it (the shipped site config's replay chain processes every event on
its own and takes each handoff event a *second* time), chain declaration
order silently matters, and a document with no `main` chain loads fine
while meaning nothing. Operators cannot tell from a rules document which
chain is the front door.

## Solution

Evaluation enters exactly one chain per event - the **entry chain**
named `main`. Every other chain is a subroutine, entered only when a
`goto`/`jump` action names it. A document without a chain named `main`
is refused at load, and `goto`/`jump` cannot target `main`; chains
nothing dispatches to are legal and never run. The shipped example rules
grow a `main` dispatcher chain so the working pattern is entry plus
dispatch, and the docs, glossary and a new ADR describe the model.

## User Stories

1. As a rule author, I want only the chain named `main` to be entered
   for each event, so that my document has one well-defined entry point
   like the firewall chains I know.
2. As a rule author, I want every other chain to run only when a
   `goto`/`jump` action names it, so that helper chains run exactly when
   I dispatch to them.
3. As a rule author, I want a jumped-to chain to run and then my current
   chain to resume, so that I can factor out shared inspection without
   losing my place.
4. As a rule author, I want a goto'd chain to run and then end my
   current chain, so that handoff semantics stay as documented.
5. As a rule author, I want a chain that nothing dispatches to to never
   run, so that drafts and shared library chains have no side effects.
6. As a rule author, I want a document without a chain named `main`
   refused at load with a clear error, so that misconfiguration fails at
   startup instead of silently processing nothing.
7. As a rule author, I want `goto`/`jump` actions targeting `main`
   refused at load with a clear error, so that I never write dead
   dispatch or hit a confusing cycle error.
8. As a rule author, I want `goto`/`jump` targets still validated to
   exist and the dispatch graph still acyclic, so that dispatch can
   never fail at runtime.
9. As a rule author, I want chain declaration order to stop mattering,
   so that reordering chains in the file cannot change behaviour.
10. As a rule author, I want a chain reached twice by different dispatch
    paths to run twice for one event, so that dispatch keeps plain
    subroutine semantics.
11. As a rule author, I want the double-dispatch pitfall documented,
    so that I can avoid feeding a shared chain's tracker twice.
12. As a rule author, I want `stop` in `main` to end processing for the
    event, so that exclusion rules gate everything below them.
13. As a rule author, I want a goto'd chain to take the triggering event
    once, not again on a top-level pass, so that handoff trackers are
    not double-fed.
14. As a rule author, I want tracker keying, leaky-bucket math and
    overflow semantics untouched by this change, so that matches mean
    exactly what they meant before.
15. As a rule author, I want terminality (`report`/`goto`/`stop` end the
    current chain only) unchanged, so that my action lists keep their
    meaning.
16. As a report consumer, I want a report's chain field to keep naming
    the chain the fired rule lives in, so that downstream tooling is
    unaffected.
17. As an operator, I want `bandage -t` to keep printing the validated
    plan in the same format, so that my validation tooling keeps
    working.
18. As an operator, I want the shipped example rules to demonstrate
    entry plus dispatch, so that I can copy a working pattern.
19. As an operator, I want the shipped site config's comments to match
    the new semantics, so that I am not misled about when replay
    inspection runs or what it costs.
20. As an operator, I want the rules guide to explain entry-chain
    semantics and the double-dispatch pitfall, so that my mental model
    matches the engine.
21. As a rule author, I want the glossary to define "entry chain"
    precisely, so that the vocabulary we all use matches the behaviour.
22. As a maintainer, I want the decision recorded as an ADR, so that
    future readers know `main`-chain entry was deliberate and what was
    rejected.
23. As a maintainer, I want benchmarks to measure the same amount of
    work after the change, so that throughput numbers stay comparable.
24. As a maintainer, I want the semantics change and the test-suite
    migration to land in one session, so that the suite is never
    red between them.

## Implementation Decisions

- The rules domain model gains the entry chain name as a constant,
  `main` - the reserved name of the entry chain in the rules format. The
  Chain concept's documentation is reworded accordingly.
- The evaluation service's event entry point runs only the entry chain,
  by name, through the same per-chain traversal that `goto`/`jump`
  already dispatches through; named dispatch, rung dispatch and
  terminality are untouched. If the entry chain is somehow absent at
  evaluation time (the loader normally guarantees it), evaluation
  returns an error rather than silently doing nothing.
- The chain-graph validation in the rules loader additionally rejects a
  document with no chain named `main` (`rules: no chain named "main"`)
  and any `goto`/`jump` action targeting `main` (an explicit error
  instead of the cycle error a reachable attempt would earn). Existing
  checks - unique chain names, resolvable targets, acyclic graph - stay.
  The entry chain therefore has in-degree 0.
- Chains unreachable from `main` remain legal and inert, like iptables
  custom chains; no reachability check.
- Dispatch is deliberately **not** deduplicated: a diamond
  (`main → A → C` and `main → C`) runs `C` twice for one event. A chain
  is a subroutine; calling it twice runs it twice. The pitfall is
  documented instead of tracked with per-event evaluator state.
- The shipped example rules gain a `main` dispatcher chain: one
  filter-less `jump` rule per topical chain. A `jump` is non-terminal,
  so the dispatcher sequences exactly like the old file order and the
  guide's worked-example YAML stays verbatim (the guide and the shipped
  file are coupled verbatim by their test).
- The `--test` plan output format is unchanged: no entry-chain
  annotation, no unreachable-chain warning (follow-up polish, out of
  scope here).
- Documentation: the rules guide gains an entry-chain semantics section
  and the double-dispatch pitfall; the project plan's pipeline/actions/
  validation wording is updated; the glossary gains the term *Entry
  chain* ("the chain entered for every event; in the rules format it is
  the chain named main"; _avoid_: base chain, subroutine, hook, regular
  chain) and *Chain* is redefined around it; ADR 0015 records the
  decision and its rejected alternatives.
- No schema, report-shape, API, ingest or tracker-math changes.

## Testing Decisions

- A good test here asserts only external behaviour: which reports a
  rules document and a stream of events produce (and which they don't),
  and which rules documents the loader accepts or refuses with which
  error - never internal call counts or traversal order inside the
  evaluator.
- Primary seam: the existing daemon-level seam used by the end-to-end
  tests - a rules document and log lines in, reports and the reports API
  out. It covers loader and evaluation together, including load refusal
  at startup, and is where the entry-chain behaviour (sub chains silent
  without dispatch, `jump` resumes, `goto` ends, single-feed handoffs,
  diamond double-dispatch) is asserted.
- Secondary seam: the existing loader-validation table (rules documents
  in, accept/reject messages out) for the fine-grained refusal messages
  (missing `main`, `goto`/`jump` targeting `main`, unchanged graph
  checks).
- The existing evaluation-service stub seam is kept for tracker-level
  cases invisible above (readings, TTLs, cache keys) - prior art: its
  replay/tracker tests - but new behaviour assertions live at the
  daemon seam.
- Prior art: the existing `goto`/`jump` end-to-end tests (their
  double-feed assertion becomes a single-feed assertion), the
  chain-graph validation table, and the escalation-ladder end-to-end
  tests.

## Out of Scope

- Multiple entry chains or a configurable entry chain name.
- Reachability validation, unreachable-chain warnings, or any `--test`
  plan annotation.
- Deduplicating dispatch to once per chain per event.
- Any change to `goto`/`jump`/`stop` terminality semantics, tracker
  math, report semantics or the reports API.

## Further Notes

- The goto end-to-end test's "tracker value = 2 (one from the
  goto-entered traversal and one from the chain's own top-level
  traversal)" becomes 1: the double-feed is the headline behaviour fix.
- Test-fixture migration is wide but mostly mechanical: every accepted
  loader fixture document needs a chain named `main`, and every app
  fixture's entry chain is renamed (multi-chain fixtures get a `main`
  that jumps to the topical chains). Bench fixtures dispatch from `main`
  so measured work stays constant.
- The semantics change and the app-suite migration land in one session;
  the shipped configs/docs ticket follows.
- Decisions 1–8 of the grilling session are captured in ADR 0015 and
  the glossary; this spec supersedes the earlier plan prose in this
  directory. The implementation tickets alongside remain the work
  breakdown.
