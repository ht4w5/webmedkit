# 03: Enrichment Filter gate

**What to build:** A Filter over events - the rule filter language -
gates the enrichment path wholly, configured as `enrichment.filter` and
defaulting to `any` like rules. Unmatched events carry default derived
fields and nothing else: no observation, no enrichment-state touch, no
stale values. Matched events are enriched as before. The Sampling rule
stays stricter than any Filter, so a widened filter can never poison
the estimate. A bad filter expression aborts startup naming the key.
Demo: scope the filter to one path prefix and watch everything outside
it flow through evaluation with default derived fields.

**Blocked by:** 01

**Status:** resolved

- [x] `enrichment.filter` compiles with the rule filter language; an invalid expression aborts startup naming the key; an unknown enrichment config key aborts startup
- [x] An unset filter defaults to `any`, like rules
- [x] Unmatched events carry default derived fields (the unknown Filesize) and never touch enrichment state - zero cache updates, zero observations
- [x] The derived-field reset runs for every event, before gating, in one place - the reservation for future enrichments
- [x] A widened filter cannot poison the estimate: `304`, `416`, error and headerless traffic never observe even when the Filter admits them
- [x] End-to-end: scoped-out traffic is enriched by nothing; scoped-in traffic behaves exactly as in tickets 01-02

Implemented on integration branch `feat/filesize-methods` (merge commit
`86d36c0`; cross-ticket code-review fixes through `4299be9`).
Context: spec `../spec.md` (Implementation Decisions 2-3).
