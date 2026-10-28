# 03: Entry-chain evaluation

**What to build:** A rules document is evaluated from its `main` chain
only — the entry chain. Every other chain runs only when a `goto`/`jump`
action dispatches to it: `jump` runs the target and resumes the caller,
`goto` runs it and ends the caller; a chain reached twice by different
dispatch paths runs twice for one event (plain subroutine semantics, the
pitfall documented rather than deduplicated). A handoff event now feeds
the target chain's tracker once — the target's own top-level pass is
gone. The loader refuses a document with no chain named `main` and any
`goto`/`jump` targeting `main`, both with clear errors; unreachable
chains stay legal and inert. The behavioural tests are rewritten to the
new model (a sub chain is silent without dispatch, dispatch semantics,
single-feed handoff, diamond double-dispatch, the load refusals), and
benchmark fixtures dispatch from `main` so measured work is unchanged.

**Blocked by:** 01, 02

**Status:** resolved

- [x] Only the chain named `main` runs per event when nothing dispatches
- [x] `jump` dispatches and resumes; `goto` dispatches and ends
- [x] A diamond dispatch runs the target chain twice for one event
- [x] A goto'd chain takes the triggering event exactly once
- [x] Load refuses documents without `main` and dispatch targeting `main`
- [x] The full suite is green and benchmarks measure the same work as before

## Answer

Implemented on integration branch `feat/main-chain-entry` (commit
`ea43e28`, merge `4e7a86e`; cross-ticket code-review fixes `4849142` +
`5d8c1d3`, merge `8f10038`). Evaluation enters only the chain named
`main` per event (`domain.EntryChain`); every other chain runs only
when a `goto`/`jump` dispatches to it — `jump` runs the target and
resumes the caller, `goto` runs it and ends the caller — through the
same per-chain traversal named dispatch already used. A handoff event
feeds the target chain's tracker once: the target's own top-level pass
is gone (the goto e2e's former double-feed assertion is now
`assertTopicRanOnce`, tracker value 1). The loader refuses documents
without a `main` chain and any `goto`/`jump` (rung dispatch included)
targeting `main`, both with explicit errors; unreachable chains stay
legal and inert. The behavioural tests are rewritten at the daemon
seam (`TestE2EOnlyTheEntryChainRunsPerEvent`,
`TestE2EGotoRunsTargetAndEndsCurrentChain`,
`TestE2EJumpRunsTargetAndResumesCurrentChain`,
`TestE2EDiamondDispatchRunsTheTargetChainTwice`, and the single-feed
handoff in `TestE2EReplayLadderRungTerminalityAndReportBehaviour`)
with the load refusals in `TestValidatesChainGraph` /
`TestParseRulesValidation`. Benchmark fixtures dispatch from `main`
(`benchDispatcherChain`), so measured work is unchanged; full suite
green.

Context: spec `../spec.md`,
`bandage/internal/domain/evaluation/evaluation.go`,
`bandage/internal/domain/rule.go`, `bandage/internal/app/graph.go`,
`bandage/internal/app/e2e_test.go`, ADR
`docs/adr/0015-main-chain-entry.md`.

Notes:
- One judgement call: a stub-seam test pins "evaluation errors when the
  entry chain is absent" (`TestEvaluateWithoutTheEntryChainErrors`) —
  a case invisible above the daemon seam, since the loader normally
  guarantees `main` exists.
- The goto/jump fixtures use `volume: 0.5` so the single feed is
  observable on one event (any event matches; the tracker value pins
  1).
- Bench: measured work is constant in tracker terms, but the dispatcher
  adds a few filter-less rule visits per event — inherent to the
  pattern, confirmed in code review.
- Glossary and ADR 0015 were pre-landed on `main` in commit `596bdd4`
  and verified against the shipped behaviour here.
