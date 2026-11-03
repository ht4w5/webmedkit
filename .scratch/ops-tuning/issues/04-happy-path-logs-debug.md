# 04: Happy-path log records drop to debug

**What to build:** The daemon stops narrating its own counters at info
level. The periodic statistics record - emitted on every sweep tick from
the same Stats snapshot the machine API serves - logs at debug level,
and the sweep-deleted record (emitted only when something was deleted)
also logs at debug. Steady-state logs carry events again, not the
daemon's self-reporting. Bad news and state changes keep their voice:
the sweep failure record stays at error and the API-started record stays
at info. A developer debugging with verbose logging still sees both
records where they always were.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The periodic statistics record emits at debug level.
- [x] The sweep-deleted record emits at debug level.
- [x] The sweep failure record still emits at error level and the
  API-started record still emits at info level.
- [x] A log-capture test pins the statistics record to debug and agrees
  it with the Stats snapshot as before; a log-capture test pins the
  sweep-deleted record to debug; all existing tests pass.

## Comments

Resolved on `integration/ops-tuning`: merged in `cd73068` (merge: 04-happy-path-logs-debug into integration/ops-tuning (happy-path log records drop to debug)); code-review fix pass merged in `1d261e2`. All checklist items are pinned by the tests named in the spec's Testing Decisions. Interactive panel behaviour remains on the manual user-story checklist per the panel's testing convention.
