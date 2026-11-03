# 02: Reports view in pprof's table skin

**What to build:** The Reports page becomes the polished report review
surface: one row per live Report - rule, target, tracker reading,
updated - in pprof's table styling (sticky right-aligned table headers,
row highlight). Clicking a row expands the full report body inline:
reason, penalty, and the whole sample. Each row keeps its Delete cell,
whose deletion withdraws the report the same way the machine API's
delete does. An empty live set renders a clear empty state, never a bare
table. The header's search box filters the rows with an unanchored,
case-insensitive regex over rule, target, and body - the same semantics
the panel's report filter always had - and an invalid regex is called
out inline instead of silently matching nothing. The right-aligned
legend shows the live report count.

**Blocked by:** 01 (Web Panel chrome - per-view pages in pprof's design)

**Status:** resolved

- [x] The review table renders in pprof's table skin with rule, target,
  tracker reading, and updated columns.
- [x] Clicking a row expands its full report body (reason, penalty,
  sample); clicking again collapses it.
- [x] Each row's Delete removes the report via the machine API and the
  row leaves the table.
- [x] An empty report set renders a clear empty-state message.
- [x] The search box narrows the rows by unanchored case-insensitive
  regex over rule, target, and body; an invalid regex shows an inline
  error and does not blank the table silently.
- [x] The legend shows the count of live reports.
- [x] Served-surface tests cover the review table's slots, the delete
  and detail affordances, and the empty-state copy; interactive
  behaviour is verified manually per the spec's user stories.

## Comments

Resolved on `integration/ops-tuning`: merged in `a42dbd2` (merge: 02-reports-view-pprof-table into integration/ops-tuning (Reports view in pprof table skin)); code-review fix pass merged in `1d261e2`. All checklist items are pinned by the tests named in the spec's Testing Decisions. Interactive panel behaviour remains on the manual user-story checklist per the panel's testing convention.
