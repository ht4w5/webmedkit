# 02: Dynamic reports - value-bound death

**What to build:** A report action without `ttl` on a tracked rule
writes a **Dynamic report**. It is served with `"dynamic": true` after
`penalty` and `expires_at: null`; fixed entries stay byte-identical
and the document `version` stays 0. A dynamic Report is alive while
its Tracker value is strictly above its recorded volume and dies at
**Underflow** - lazily, at document generation and sweep time, with no
timer. The next overflow after death re-flags a fresh Report at the
accumulation's current stance. Trigger suppression becomes
mode-aware: fixed entries by expiry (unchanged), dynamic entries by
value-based liveness against the tracker record; the override gate
(strictly higher recorded volume) and the single write path are
untouched. Load refuses a `ttl`-less report action on a tracker-less
rule and on a zero-leak Tracker.

**Blocked by:** 01

**Status:** resolved

- [x] A `ttl`-less report action on a tracked rule writes a dynamic
      Report; a `ttl`-bearing one keeps today's fixed semantics
      byte-identically.
- [x] The served document shows dynamic entries with `dynamic: true`
      (key after `penalty`) and `expires_at: null`; a second pinned
      example asserts the exact serialization; the existing canonical
      example is byte-identical.
- [x] A dynamic Report dies when its tracker value decays to its
      recorded volume (strict boundary) and is re-flagged on the next
      overflow; with traffic flowing, renewed overflow re-flags within
      an event.
- [x] Suppression: while alive, Triggers write nothing but actions
      keep running; a strictly higher rung overrides in place; `created_at`
      frozen; a failed write retries on the next matching event.
- [x] Load refuses `ttl`-less report actions on tracker-less rules and
      zero-leak Trackers (each error naming the rule and the reason).
- [x] Persistence carries the dynamic flag (guarded column add with
      default 0 = fixed; `expires_at` sentinel for dynamic rows; the
      live-rows query serves dynamic rows); an old-format database
      opens and serves correctly after upgrade.
- [x] `bandage -t` prints the lifecycle of every report action.
- [x] The report contract doc's lifecycle and entry sections are
      amended pre-release.
- [x] Behavioral coverage rides the daemon seam (ingest syslog, clock
      injected, assert the document); validation rides the loader/plan
      seam.

Implemented on branch `ticket/02-dynamic-report-death` (behavior,
validation and persistence in `e7dc58b`, report contract doc in
`26ed934`), targeting integration branch `integration/dynamic-reports`.
Death is value-based at document generation (a dead dynamic Report is
absent and its live-set entry dies, so the next overflow writes within
an event) and at sweep time (the dead row goes; `DeleteExpired` never
eats dynamic rows - checklist-silent, taken from the spec's Persistence
decision). `Rebuild` seeds suppression from fixed rows only: no Report
outlives the volatile tracker state, so ticket 05's startup sweep only
deletes the rows. `grace` (03), Downgrade (04) and the startup sweep and
rollback tests (05) are out of this ticket; the strict line is dead at
`value <= volume`.
Context: spec `../spec.md` (Implementation Decisions - Two lifecycle
modes, Dynamic report liveness, Trigger suppression, Report document,
Persistence; Testing Decisions).
