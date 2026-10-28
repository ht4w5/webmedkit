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

**Status:** ready-for-agent

- [ ] Only the chain named `main` runs per event when nothing dispatches
- [ ] `jump` dispatches and resumes; `goto` dispatches and ends
- [ ] A diamond dispatch runs the target chain twice for one event
- [ ] A goto'd chain takes the triggering event exactly once
- [ ] Load refuses documents without `main` and dispatch targeting `main`
- [ ] The full suite is green and benchmarks measure the same work as before
