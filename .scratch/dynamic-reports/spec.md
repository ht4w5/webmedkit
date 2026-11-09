# Spec: dynamic reports - value-bound report lifecycle

Status: ready-for-agent

## Problem Statement

A report action's `ttl` is a wall-clock lifetime, and that is the wrong
shape for penalties that track traffic behavior. A bandwidth Tracker
overflowing a volume recommends `limit-bandwidth`; the Report lives for
its `ttl` no matter what the Target does next. When the accumulation
leaks back below the volume - the client stopped, the abuse is over -
the Report (and the ban rule downstream tooling derives from it) stays
enforced until the clock runs out. Enforcement outlives its
justification. Operators want a Report to live exactly as long as its
justification does: while the tracker value is above the volume.

## Solution

A **Dynamic report**: a report action with **no `ttl`** on a tracked
rule writes one. It is alive while its Tracker's value exceeds its
death line - its recorded volume minus a margin - and dies at
**Underflow**, when the value decays to that line. An optional `grace`
duration on the report action widens the line (compiled to a margin of
`leak × grace`) so a brief pause does not flap enforcement. On a
ladder, a dynamic Report **Downgrades** as the value decays: it
restates itself to the highest rung still exceeded - reason, Penalty,
Sample and update time moving down, `created_at` frozen - and dies when
no rung remains exceeded. A report action that writes `ttl` keeps
today's fixed semantics exactly. Fixed and dynamic report actions mix
on one ladder: the policy attaches to the rung that wrote the stance.

Supporting changes: dynamic Reports are deleted at daemon startup (no
Report outlives the volatile tracker state that justifies it); the
report document marks dynamic entries with a `dynamic` field and
`expires_at: null`; and the replay volume methods
(`constant`, `clamped-linear`, `inverse-square`, `step`) are removed
entirely - replay's volume becomes a plain scalar in download
equivalents.

## User Stories

1. As a rules author, I want to omit `ttl` on a tracked rule's report
   action, so that the Report lives exactly as long as the Tracker
   value overflows its volume.
2. As a rules author, I want `ttl` to keep its current meaning when
   written, so that existing rules files behave byte-identically.
3. As a rules author, I want `ttl` and `grace` to be mutually
   exclusive on a report action, so that every stance carries exactly
   one lifecycle policy.
4. As a rules author, I want a `grace` duration on a dynamic report
   action, so that a short decay below the line does not withdraw and
   re-flag enforcement in a flap.
5. As a rules author, I want load to refuse a report action without
   `ttl` on a tracker-less rule, so that a tripwire report can never
   be unbounded.
6. As a rules author, I want load to refuse a `ttl`-less report action
   on a zero-leak Tracker, so that a bucket that can never underflow
   cannot declare a dynamic report.
7. As a rules author, I want `bandage -t` to print `grace=` beside
   `ttl=`, so that every lifecycle policy is auditable before deploy.
8. As a rules author, I want to mix fixed and dynamic report actions on
   one ladder, so that e.g. a ban rung is time-capped while a throttle
   rung tracks live traffic.
9. As a rules author, I want each stance to adopt the policy of the
   rung that wrote it - on Trigger override and on Downgrade alike -
   so that lifecycle is always the writing action's.
10. As a security operator, I want a dynamic Report to die when its
    Tracker value decays to the death line, so that enforcement is
    withdrawn the moment the behavior is gone.
11. As a security operator, I want the death line to be volume minus
    `leak × grace`, so that enforcement survives brief pauses in
    otherwise sustained abuse.
12. As a security operator, I want the boundary strict - alive only
    while the value is strictly above the death line - so that
    Underflow mirrors Overflow's strictness.
13. As a security operator, I want a ladder Report to Downgrade to the
    highest rung still exceeded as the value decays, so that the
    Penalty always matches the accumulation's current stance - a ban
    becomes a throttle before it disappears.
14. As a security operator, I want a Downgraded stance to carry the
    lower rung's reason and Penalty with a fresh update time, so that
    the Sample (from the climb event, which crossed every lower rung)
    keeps backing the stance.
15. As a security operator, I want `created_at` to stay frozen across
    Downgrades, so that first-flagged provenance survives the whole
    lifetime.
16. As a security operator, I want a Downgrade onto a fixed rung to
    adopt that rung's `ttl` from the downgrade write, so that the
    stance's bound is the rung author's declared one.
17. As a security operator, I want a dynamic Report to die when no rung
    remains exceeded, so that no stance outlives its justification.
18. As a security operator, I want Trigger suppression to be unchanged
    while a Report is alive - one Report per (rule, Target), strictly
    higher rung overrides in place - so that the write path stays
    single-sourced.
19. As a splint operator, I want dead dynamic Reports to simply be
    absent from `GET /api/reports`, so that enforcement is withdrawn
    at my next sync with no new handling.
20. As a report consumer, I want dynamic entries marked with a
    `dynamic` field and `expires_at: null`, so that "no scheduled
    withdrawal" is distinguishable from a timestamp.
21. As a report consumer, I want fixed entries byte-identical to today,
    so that my existing integration is untouched.
22. As a report consumer, I want the document `version` to stay `0`,
    so that the additive change needs no consumer migration.
23. As a daemon operator, I want all dynamic Reports deleted at
    startup, so that no Report outlives the volatile tracker state
    that justifies it.
24. As a daemon operator, I want one summary log line for the startup
    sweep and `debug`-level lines for Downgrades and dynamic deaths,
    so that constant lifecycle acts never flood the log.
25. As a daemon operator, I want an existing report database to open
    unchanged after the upgrade, so that deployment is a plain binary
    swap.
26. As a daemon operator, I want rollback to an older binary to be
    safe, so that dynamic rows are merely not served, never corrupted.
27. As a rules author, I want replay's volume to be a plain scalar in
    download equivalents, so that I state tolerated downloads without
    a volume method.
28. As a rules author, I want a rules file carrying a volume-method
    block to fail load with an error naming the removal, so that the
    migration is discoverable at `bandage -t` time.
29. As a web panel operator, I want dynamic Reports on the reports
    page with their null expiry, so that currently-justified
    enforcement is visible.
30. As a security operator, I want a fresh Report written on the next
    overflow after a dynamic Report dies, so that renewed abuse
    re-flags at the accumulation's current stance.

## Implementation Decisions

- **Two lifecycle modes, one per report action (Model M).** A report
  action with `ttl` is exactly today's fixed semantics - immune to
  decay, dies at `updated_at + ttl` - byte-identical for every
  existing rules file. A report action without `ttl` writes a Dynamic
  report: alive while its Tracker value exceeds its death line. The
  policy attaches to the report action that last wrote or restated the
  stance (Trigger override and Downgrade alike).
- **Report action grammar**: `reason`, optional `ttl`, optional
  `grace`, `penalty`, `rate`. `ttl` and `grace` are mutually
  exclusive; both written is a load error. `grace` parses like `ttl`
  (Go duration strings plus `d` for days). At rule build, `grace`
  compiles to a margin of `leak × grace`; the death line is
  `volume − margin`. Absent `grace` means the strict line: dead at
  `value <= volume`.
- **Load validation additions**: a report action on a tracker-less
  rule must carry `ttl`; a report action on a zero-leak Tracker must
  carry `ttl`; `ttl` and `grace` together is refused; a volume-method
  block anywhere is refused with an error naming the removal. The
  `bandage -t` plan prints `grace=` beside `ttl=` on every report
  line.
- **Dynamic report liveness**: alive iff the Tracker record named in
  the Report's reading (the writing key) holds a value strictly above
  the death line of the Report's recorded volume. Lifecycle acts are
  materialized lazily, at document generation and at sweep time,
  against live tracker state; nothing runs between events and no timer
  exists. The fixed-mode acts (expiry) keep their current lazy
  mechanics.
- **Downgrade** (dynamic stances on a ladder only): when the value
  decays to the current stance's death line and some lower rung is
  still exceeded, the Report restates itself to the highest rung
  still exceeded - reason, Penalty, Sample, tracker reading and
  `updated_at` move, `created_at` stays frozen - the margin guarding
  the downgrade line too, so stances do not flap between rungs at the
  boundary. The restated stance adopts the target rung's policy: a
  fixed rung's `ttl` runs from the downgrade write. The Report dies
  when no rung remains exceeded. Fixed stances never decay-interact:
  no Downgrade, no underflow death.
- **Trigger suppression**: the alive-gate becomes mode-aware - fixed
  entries by their expiry, dynamic entries by value-based liveness
  against the tracker record - while the override gate (strictly
  higher recorded volume) and the single-writer path are unchanged.
  A failed write remains not a Trigger outcome.
- **Report document (version 0)**: dynamic entries append a
  `"dynamic": true` field after `penalty` and render `expires_at` as
  `null`; fixed entries stay byte-identical and the existing canonical
  example remains pinned unchanged. A second pinned example covers a
  dynamic entry. The contract prose is amended pre-release; `version`
  stays `0` under the documented additive policy. Downstream tooling
  (splint) needs no change: withdrawal is by absence at sync.
- **Persistence**: the report table gains a `dynamic` column
  (`NOT NULL DEFAULT 0`) added by one guarded `ALTER TABLE` at open -
  the schema is create-if-not-exists only, and old rows are fixed
  reports by construction, so the default is exact, not a guess.
  Dynamic rows store a `0` sentinel in `expires_at` (its `NOT NULL`
  cannot be relaxed in place); the live-rows query becomes
  "unexpired or dynamic". The persisted body gains the `dynamic` flag.
  Rollback to an older binary is safe: the extra column is ignored and
  sentinel rows fall outside its expiry filter - not served, not
  rewritten, and cleanly overwritten if the same (rule, Target) is
  re-flagged.
- **Startup sweep**: all dynamic reports are deleted before the
  live-report set is rebuilt for suppression; one summary log line
  reports the count. This amends the durable-reports decision: reports
  survive restarts only while fixed. Accepted consequence on record:
  re-flagging after a restart requires re-accumulation past volume
  from an empty bucket - a burst-then-pause offender gains amnesty
  from a restart during the pause, and a crash loop is an amnesty
  machine - accepted because restarts are seldom.
- **Volume-method removal**: the volume methods (`constant`,
  `clamped-linear`, `inverse-square`, `step`) and their computation
  seam are deleted; replay's volume is a plain scalar in download
  equivalents (the ladder `at` quantity language is unchanged). The
  per-event volume re-derivation in evaluation goes with it. Side
  benefit: a Tracker's volume is now constant per rule, so death lines
  are well-defined. This is a breaking rules-format change: a file
  with a volume-method block fails load with an error naming the
  removal. Land as a separate commit in the same change.
- **Vocabulary** (glossary updates land with this change): **Dynamic
  report**, **Grace**, **Downgrade**, **Underflow** as new terms; the
  Report and Trigger entries are reworded ("alive until expiry,
  Underflow of its recorded volume, or deletion"); the **Volume
  method** entry is deleted. Two ADRs record the decisions: a
  dynamic-report lifecycle ADR (amending the trigger-semantics,
  durable-reports and escalation-ladder ADRs - the ladder ADR's "no
  downgrade and no rewrite" narrows to "no downgrade from Triggers";
  decay-driven Downgrade is its inverse) and a volume-method removal
  ADR.
- **Observability**: Downgrades and dynamic deaths log at `debug`; the
  startup sweep logs one summary line. Nothing is added to Stats or
  Metrics.

## Testing Decisions

A good test asserts external behavior only: drive Events into the
assembled system, advance the injected clock, and assert what the
system serves or refuses - never internal caches, bookkeeping
structures, or log text (lifecycle acts are deliberately unlogged
above `debug`).

- **Primary seam (existing)**: the assembled daemon driven by raw
  nginx-JSON syslog messages at a real ingest listener, asserted
  through `GET /api/reports` JSON, on the injected wall clock - the
  seam the daemon tests already declare. Prior art: the trigger,
  suppression and escalation tests. Everything behavioral rides this
  seam: dynamic death at the strict boundary, the grace margin,
  Downgrade through mixed ladders, fixed stances ignoring decay,
  suppression and override across modes, re-flagging after death, and
  the startup sweep (assemble, ingest, re-assemble over the same
  database, assert the document).
- **Rules-load seam (existing)**: the loader and its `bandage -t`
  plan - validation refusals (`ttl`+`grace`, tracker-less without
  `ttl`, zero-leak without `ttl`, volume-method block), scalar replay
  volume parsing, and `grace=` in the plan output. Prior art: the
  rules parsing/validation and plan tests.
- **Persistence seam (existing)**: migration and rollback only - an
  old-format database opens with the column added and old rows fixed;
  a new-format database is served (and dynamic rows skipped) by the
  old query shape. Prior art: the sqlite repository tests.
- **Pinned document test (existing)**: the canonical example stays
  byte-identical; a second pinned example asserts a dynamic entry's
  exact serialization (`dynamic` key position, `expires_at: null`).

Modules under test: the rules loader, the report service (lifecycle
and suppression), the daemon startup path, the report document
builder, and the sqlite report repository (schema migration).

## Out of Scope

- splint changes - it parses and ignores `expires_at`, and withdrawal
  is already absence-driven.
- Anti-flap beyond the grace margin (sustained-below windows, true
  grace state on the record).
- Persisting tracker records or seeding them from report snapshots at
  restart (explicitly rejected: restarts are seldom).
- Stats counters or Metrics series for lifecycle acts (series can
  never be removed once added).
- Any change to tracker cache TTLs, LRU eviction, or the leaky-bucket
  math.
- Filesize and Sampling-rule behavior; the size-0-means-unknown
  convention.
- A document version bump - the contract stays `version` 0.

## Further Notes

- **Grace versus margin**: under linear decay the two are the same
  quantity - dying "grace after underflow" is dying at
  `volume − leak × grace` - so grace compiles to a margin and no
  lifecycle state exists. The one behavioral difference from a
  "true" grace window: if trickle traffic parks the value in the band
  just below the volume, the margin keeps the Report alive; a true
  grace would kill it. Accepted; documented in the ADR.
- **Restart amnesty** is the deliberate price of volatile trackers
  (the durable-reports ADR already accepts restarts as a free pass);
  the sweep makes it apply to dynamic reports with eyes open. If
  practice shows crash loops matter, the mitigation is the rejected
  snapshot-seeding fallback, not more state.
- **Work order**: the volume-method removal commit first (it makes
  volumes constant), then the dynamic lifecycle, then docs/glossary/
  ADRs. The two concerns ship in one change.
