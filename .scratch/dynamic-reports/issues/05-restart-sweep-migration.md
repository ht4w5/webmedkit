# 05: Restart sweep and migration safety

**What to build:** On daemon startup, all dynamic reports are deleted
before the live-report set is rebuilt for suppression - no Report
outlives the volatile tracker state that justifies it - with one
summary log line counting the sweep. Rollback is safe: an older binary
against a new-format database simply does not serve dynamic rows (the
extra column ignored, sentinel rows outside its expiry filter), never
corrupts them, and cleanly overwrites one if the same (rule, Target)
is re-flagged.

**Blocked by:** 02

**Status:** resolved

- [x] Restarting a daemon over an existing database removes every
      dynamic Report and keeps every fixed Report; suppression is
      rebuilt from the fixed rows only (a re-flagged target writes
      fresh).
- [x] The startup sweep logs one summary line (count), nothing
      per-report above `debug`.
- [x] An old-format database opens after upgrade: column added
      guarded, old rows are fixed reports by construction.
- [x] The old query shape (unexpired-only) demonstrably skips dynamic
      rows - a rollback test proves the old-binary path serves the
      fixed set and leaves dynamic rows untouched.
- [x] The restart-amnesty consequence is stated where the durable-
      reports decision is recorded: re-flagging after restart requires
      re-accumulation past volume from an empty bucket.
- [x] Sweep and migration ride the daemon seam (assemble, ingest,
      re-assemble over the same database, assert the document) and the
      persistence seam respectively.

Implemented on branch `ticket/05-restart-sweep-migration` (sweep in
`ed9b358`, persistence-seam migration/rollback pins in `b96247b`,
durable-reports ADR amendment in `c447611`), targeting integration
branch `integration/dynamic-reports`. The startup sweep runs in
`Daemon.Start` before `Rebuild`: `Service.DeleteDynamic` deletes the
rows and drops the deleted dynamic pairs from the live set, reported
as one summary log line (info, count) with nothing per-report above
`debug`. Restart amnesty is on record in ADR 0002 - re-flagging after
a restart re-accumulates past volume from an empty bucket. Rollback
safety rides the literal old binary shapes at the persistence seam:
the old query shape serves the fixed set and leaves dynamic rows
untouched, the old upsert cleanly overwrites one on re-flag (fresh
report where the dynamic row stood, row identity preserved), and the
column addition is guarded across reopen. Two rollback corners on
record, neither corrupting a served row: an old binary's sweeper eats
sentinel rows within a tick (it cannot serve them; equivalent to the
sweep), and an old-binary re-flag leaves the stale `dynamic` column on
the overwritten fixed row (served past its expiry until the next
startup sweep removes the row).
Context: spec `../spec.md` (Implementation Decisions - Persistence,
Startup sweep, Observability; User stories 23-26; Testing Decisions -
Persistence seam).
