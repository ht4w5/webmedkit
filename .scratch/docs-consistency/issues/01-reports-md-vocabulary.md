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

**Status:** ready-for-agent
