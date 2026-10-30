# 01: Namespaced reasons on reports, end to end

**What to build:** A report written by a triggered report action carries the
authored code namespaced under the daemon's program name - `bandage.<code>` -
composed from the build identity the daemon already receives, at the moment
the report is created. The namespaced form is what the report table stores and
what `GET /reports` serves. An escalation override keeps its in-place
semantics and carries the overriding rung's namespaced code. The report
document's canonical example is updated and the served response reproduces it
byte for byte. A daemon driven under a different program name serves reasons
under that name, pinning that the namespace follows the program name. Every
existing assertion on a reason string is updated in this ticket so the suite
lands green.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A created report's reason is `<program name>.<code>`; the report action
      keeps carrying the bare authored code
- [x] The prefix is composed from the build identity at report creation only -
      not at rules load, not at serialization - as a plain join (no
      re-prefixing)
- [x] A daemon driven under another program name serves reasons under that
      name (ADR 0017's derivation, pinned behaviorally)
- [x] An escalation override writes the overriding rung's namespaced code in
      place; identity, code sharing across rules, and all other fields are
      unchanged
- [x] `GET /reports` serves the namespaced form; the response reproduces the
      canonical example of the report document contract byte for byte, the
      example showing a namespaced reason
- [x] Persistence round-trips the namespaced form unchanged; no migration or
      read-path normalization
- [x] Rules loading is untouched: bare codes pass, dotted codes are refused
- [x] Every existing suite assertion on a reason string is updated here; the
      full suite is green

Prior art for the tests: the end-to-end suite's report assertions through the
served report document, the report-document byte-pin test, and the rules
suite's canary build identity (for the derivation case).

## Answer

Implemented on integration branch `feat/reason-prefix` (commits
`63cab56`, `4ab4191`, `af3117e`, merge `651dd05`; cross-ticket
code-review fix `d730581`, merge `b0eb730`). A report written by a
triggered report action carries the authored code namespaced under the
daemon's program name: `evaluation.trigger` composes
`domain.NamespacedReason(progName, action.Reason)` at the single write
path to the report table, the bare code travelling unchanged in the
report action and the rules DSL. The name is threaded as a plain string
from the build identity the daemon receives
(`identity.Build.Name()`), so the domain core stays stdlib-only.
`GET /reports` serves the namespaced form and persistence round-trips
it verbatim (pinned at the driven-daemon restart seam,
`TestE2EEscalationOverrideGuardSurvivesRestart`); the escalation
override writes the overriding rung's namespaced code in place
(`TestE2EEscalationHigherRungOverridesReportInPlace` walks
`bandage.throttle` -> `bandage.squeeze` -> `bandage.ban`). The
derivation case (`TestE2EReportReasonNamespaceFollowsThePassedBuildName`)
drives a daemon under `identity.For("bandage-canary")` and serves
`bandage-canary.<code>`. `docs/reports.md` publishes the wire pattern
`^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*$` at version 0, the
canonical example showing `"reason": "bandage.egress-overflow"` --
pinned byte for byte by the report-document test. Every broken reason
assertion across the e2e, escalation and replay-ladder suites was
updated to the namespaced form; loader assertions stay bare.

Context: spec `../spec.md`, `bandage/internal/domain/report.go`,
`bandage/internal/domain/evaluation/evaluation.go`,
`bandage/internal/app/daemon.go`, `bandage/internal/app/e2e_test.go`,
`docs/reports.md`, ADRs `docs/adr/0016-report-reasons-are-namespaced.md`
and `docs/adr/0017-reason-prefix-comes-from-the-progname.md`.

Notes:
- The join lives in `domain.NamespacedReason` (added in the review fix
  pass) so the plan and the report print the same reason string (user
  story 13); composition still happens only at report creation and at
  the plan preview.
- The loader-table case "reason with a dot rejected" pins ticket 01's
  "bare codes pass, dotted codes are refused"; ticket 02 added the
  failing/passing pair at the plan seam -- two existing seams, one
  ticket each, both kept (review disposition O1).
- Persistence round-trip is pinned at the restart e2e seam rather than
  the persistence suite (spec: persistence suites are expectation
  fallout, not feature specification).
