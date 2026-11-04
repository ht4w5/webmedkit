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

**Status:** ready-for-agent

- [ ] The rule counters table renders through a shared counter-table mechanism, not a rule-specific one
- [ ] Table shape stays single-sourced from the served header row (message-row span derived, not hard-coded)
- [ ] The explicit empty state and shared search-box narrowing behave exactly as the existing panel pins specify
- [ ] Every existing panel pin passes unchanged: no operator-visible change on the Stats page
