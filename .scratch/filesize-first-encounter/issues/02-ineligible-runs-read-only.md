# 02: Ineligible runs are read-only

**What to build:** An enrichment run without a successful observation
never creates a file-size record: the file-size cache port gains a
read-only lookup beside its atomic update, and a record exists exactly
for paths with at least one observation. Such a run still enriches with
the current estimate when one exists, else leaves the size at its
unknown default (stale values overwritten). Scanner noise - 404 sweeps
and probes flowing through a rule with an enrich action - can no longer
fill the file-size cache with empty records and exert eviction pressure
on real estimates.

**Blocked by:** 01

**Status:** resolved

- [x] The file-size cache port has a read-only lookup; the atomic
      update remains the only writer
- [x] A run without a successful observation creates no record and
      observes nothing
- [x] Such a run enriches with the current estimate on an observed
      path, and leaves the size unknown on an unobserved one
- [x] The eligibility-table suite gains a "no record created" case
      alongside its existing expectations

Implemented on integration branch `integration/filesize-first-encounter`
(merge commit `2c6ddbe`; code-review refactors through `19010f4`). The
e2e stats pins were updated accordingly: read-only lookups count in
`tracker_cache` gets/misses, never sets.
Context: spec `../spec.md` (Implementation Decisions; Out of Scope).
