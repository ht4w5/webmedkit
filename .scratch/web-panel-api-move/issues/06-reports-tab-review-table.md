# 06: Reports tab: report review table with expandable bodies

**What to build:** The Web Panel's Reports tab lists every live Report
fetched from `/api/reports` - one row per Report with its rule, Target,
tracker reading, and updated time. The operator clicks a row to expand the
report's full body pretty-printed - Reason, Penalty, Sample, expiry -
reviewing a report without leaving the page or reading raw JSON. Expired
reports cannot appear (the repository never returns them), so the list is
exactly what a consumer would fetch.

**Blocked by:** 05 - Web panel shell with Statistic tab at the web root

**Status:** resolved

- [x] The Reports tab lists one row per live Report with rule, Target, tracker reading, and updated time.
- [x] A row expands to show the report body pretty-printed, including Reason, Penalty, Sample, and expiry.
- [x] A row collapses again; expanding one row does not disturb others.
- [x] The list reflects the current report set on manual refresh.
- [x] An empty report set renders a clear empty state, not a blank table.

## Answer

Implemented on `ticket/wam-06-reports-tab-review-table` (commit
`e3d5cba`, fast-forwarded onto `integration/web-panel-api-move`). The
Reports tab lists every live Report from `GET /api/reports` - one row
per Report with rule, Target, tracker reading, updated time. Clicking
a row expands it to the full report entry pretty-printed (Reason,
Penalty, Sample, expiry); clicking again collapses; expanding one row
never disturbs others. Manual refresh re-renders the current report
set (expired reports never reach the wire - the repository never
returns them). An empty set renders a clear empty-state row. Serving
tests cover the table/expand/refresh copy at the served surface;
interactive behavior is manual per the spec's Testing Decisions.
Reviewed in the fix pass `62e3efa`.
