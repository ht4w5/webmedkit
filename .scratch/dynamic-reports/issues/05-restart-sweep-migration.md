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

**Status:** ready-for-agent

- [ ] Restarting a daemon over an existing database removes every
      dynamic Report and keeps every fixed Report; suppression is
      rebuilt from the fixed rows only (a re-flagged target writes
      fresh).
- [ ] The startup sweep logs one summary line (count), nothing
      per-report above `debug`.
- [ ] An old-format database opens after upgrade: column added
      guarded, old rows are fixed reports by construction.
- [ ] The old query shape (unexpired-only) demonstrably skips dynamic
      rows - a rollback test proves the old-binary path serves the
      fixed set and leaves dynamic rows untouched.
- [ ] The restart-amnesty consequence is stated where the durable-
      reports decision is recorded: re-flagging after restart requires
      re-accumulation past volume from an empty bucket.
- [ ] Sweep and migration ride the daemon seam (assemble, ingest,
      re-assemble over the same database, assert the document) and the
      persistence seam respectively.
