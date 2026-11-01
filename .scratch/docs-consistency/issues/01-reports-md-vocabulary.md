# 01: docs/reports.md vocabulary and `track` enum

**What to build:** `docs/reports.md` only. (a) Reword report-side uses
of "record" to "report" / "report entry" per GLOSSARY's Tracker record
scoping: line 5 "every record field", 22 "live report records",
`## Report record` (:29), 31 "One record per (rule, target)" and "the
last key of a record", 40-41 "when the record was first/last written",
45 "every canonical record", 168 "the record's end". Keep line 88 "The
tracker's record is never emptied" - precisely correct under the new
term. (b) Line 81-82: "the evidence always backs the penalty" ->
"the sample always backs the penalty". (c) Line 51 `track` field:
document the empty case - a tracker-less rule's report carries an empty
`track` and a zeroed reading. (d) Line 54 "the overflow threshold
`value` exceeded" stays as-is (glossary now says "exceeds").

**Blocked by:** None (can start immediately)

**Status:** resolved

## Comments

Resolved on branch `ticket/dc-01-reports-md-vocabulary`. Report-side
"record" is now "report" / "report entry" throughout `docs/reports.md`
(the section is `## Report entry`); "The tracker's record is never
emptied" stays, as does "the overflow threshold `value` exceeded".
The `track` row documents the tracker-less case (empty `""`, zeroed
reading), and the lifecycle sentence now reads "the sample always backs
the penalty". Two sites beyond the list moved under the same glossary
scoping: the pre-penalty paragraph ("Records written before the penalty
existed" -> "Report entries written ...") and the quoting comment in
`splint/internal/domain/wire.go` (follows the renamed heading,
comment-only, like ticket 02's `updater.go` echo). No test pinned the
changed prose; the byte-pinned fenced example is untouched.

Context: spec `../spec.md` (ticket 01, Q10/Q12/Q13/Q7; glossary:
unqualified "record" means tracker state, never a Report; "exceeds" is
canonical and `docs/reports.md:54` stays).
