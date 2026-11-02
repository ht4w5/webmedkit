# 07: Per-row report delete with enforcement-withdrawal confirmation

**What to build:** Each report row offers a delete button. The first
click asks for confirmation; the confirmation copy states that the
enforcement derived from the report will be withdrawn at the enforcing
consumer's next sync - naming no specific consumer, since deployments may
run any ban-rule consumer. Confirming deletes the Report by its identity
(rule, Target) through the API and refetches the list. The operator doing
report review can remove a stale or wrong Report and sees it disappear.

**Blocked by:** 06 - Reports tab: report review table with expandable bodies

**Status:** resolved

- [x] Each row has a delete button; the first click is a confirmation step, not an immediate delete.
- [x] The confirmation copy states enforcement is withdrawn at the enforcing consumer's next sync and names no specific consumer.
- [x] Confirming deletes exactly the reviewed Report by (rule, Target); cancelling deletes nothing.
- [x] After deletion the list refetches and the Report is gone; the API delete is also verifiable at the HTTP seam.
- [x] The glossary's Report entry records that deletion withdraws the report's enforcement at the enforcing consumer's next sync.

## Answer

Implemented on `ticket/wam-07-report-delete-confirm` (commit
`8993bc8`, fast-forwarded onto `integration/web-panel-api-move`). Each
row has a Delete button (its own column, stopPropagation so it never
toggles row expansion); the first click swaps in a confirmation block
whose copy states that the enforcement derived from the report will be
withdrawn at the enforcing consumer's next sync and names no specific
consumer. Confirm deletes exactly the reviewed Report by (rule,
Target) via `DELETE /api/reports` and refetches; Cancel deletes
nothing. The API delete stays pinned at the HTTP seam
(`TestAPIReportsDeleteRemovesByRuleAndTarget`). GLOSSARY's Report
entry records the deletion semantics. Reviewed in the fix pass
`62e3efa` (confirm copy pinned in served-text tests).
