# 01: Prefactor — one implicit rung per match

**What to build:** No behaviour change. A Match resolves to "the rungs it
fired" — each rung a volume threshold plus an ordered action list — where
a rule without a ladder is exactly one implicit rung carrying its volume
and its action list. Dispatch, terminality and the trigger-time reading
all flow through that one shape, so the ladder becomes an additive case
instead of a rewrite. This is the prefactor that makes the change easy
before the easy change.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Every existing test passes with no edits to any test
- [x] A single-volume rule still Matches on filter plus Overflow, runs
      its full action list in order, and ends chain traversal exactly
      when its action list carries report/goto/stop
- [x] The trigger-time reading still records the rule's volume as the
      reading's volume, and log actions still report the reading
- [x] Tracker-less rules still match on the filter alone
- [x] goto/jump/stop traversal and report suppression behave exactly as
      before (verified by the existing end-to-end suites)

Implemented on integration branch `feat/escalations` (commit `8c6c84c`;
cross-ticket code-review fix pass through `129c5ea`). The prefactor
introduced `domain.Escalation` — a rung carrying `At`, an ordered action
list and static `Terminal()` — and `Rule.Rungs` resolving a match to the
rungs it fired, where a rule without a ladder is exactly one implicit
rung carrying its volume and action list. Evaluation dispatches per rung
and terminality is asked of the fired rungs; every existing suite passed
with zero test edits.

Context: spec `../spec.md` (Implementation Decisions 2-3); the shape
tickets 03-05 extend.
