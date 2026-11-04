# 01: Prefactor - shared counter-table rendering on the Stats page

**What to build:** The Web Panel's Stats page renders its rule
counters table through one reusable counter-table mechanism: table
shape derived from the served header (the existing single-source-of-
shape guarantee survives), an explicit empty-state row, and shared
search-box narrowing. Nothing operator-visible changes - the rule
counters table looks and behaves exactly as it does today - but the
rendering is now one mechanism a second counter table can adopt
instead of duplicating the machinery.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The rule counters table renders through a shared counter-table mechanism, not a rule-specific one
- [x] Table shape stays single-sourced from the served header row (message-row span derived, not hard-coded)
- [x] The explicit empty state and shared search-box narrowing behave exactly as the existing panel pins specify
- [x] Every existing panel pin passes unchanged: no operator-visible change on the Stats page

## Answer

Implemented on `ticket/ea-01-shared-counter-table` (commit `00e695d`),
merged into `integration/enrichment-actions` (`a1492df`).
`web/common.js` gains one reusable counter-table mechanism:
`renderCounterTable(table, counters, match)` plus a per-table config
(row slot, row builder, search-fields narrowing predicate, empty-state
copy, header-derived `columns` shape) and a `COUNTER_TABLES` registry
driving generic counter-block dispatch in place of the
`RULE_COUNTERS_SLOT` special case; `renderRuleCounters` becomes a thin
adoption wrapper. New pin
`TestRuleCountersTableRendersThroughTheSharedCounterTableMechanism`;
all existing panel pins pass unchanged - no operator-visible change.
Review fix pass (`1221b45`): the mechanism also absorbs the fetch/apply
wiring (`store`/`apply` per config), so a later counter table is one
config entry. Ticket 03 adopts it for the enrichment table.

Context: spec `../spec.md` (Testing Decisions), ticket
`03-enrichment-counters-surfaces.md` (the consumer).

## Comments
