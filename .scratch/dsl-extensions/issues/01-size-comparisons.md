# 01: Size comparisons end to end

**What to build:** A rules author writes `size >= 100m` / `size <= 1m`
and their rule scopes by the Filesize estimate: the DSL accepts the
comparison forms with binary size units like `sent`, matches are
inclusive at the boundary, and an event whose size is unknown is never
claimed by a comparison. Malformed forms abort `bandage -t` with the
DSL's position-carrying errors, and size units are refused everywhere
they do not belong with an error naming both unit-carrying keywords.
The rules docs carry the new predicates, the unit note, and the
`size`-in-`enrichment.filter` trap (the gate runs before the estimate is
looked up: a `size` comparison there disables enrichment - documented,
not rejected at load).

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `size >= N` / `size <= N` accept N as a byte count or a binary-unit size (`k`, `m`, `g`), like `sent`
- [x] Matches are inclusive at the boundary and consider only events whose Filesize is known; unknown-size events never match a comparison
- [x] Malformed forms (bare `size 5`, a missing or overflowing limit) abort load with 1-based positions, like every DSL error
- [x] Size units are accepted only after `sent` and `size`; the rejection error names both keywords (e.g. `status 200k` still fails)
- [x] New Filter implementations for at-least / at-most with validating constructors rejecting negative limits
- [x] Rules-file format version stays 0; existing rules files parse identically
- [x] Rules docs list the predicates, the unit note, and the enrichment-gate trap
- [x] Tests at the existing Filter seam (Parse -> Match tables), mirroring the `sent` tables

Context: spec `../spec.md`, ADR `0024` (size 0 means unknown), ADR
`0019`, ADR `0021`.

Implemented on integration branch `integration/dsl-extensions` (merge
commit `ffdd65d`; cross-ticket code-review fixes through `5af6d25`,
incl. the shared `parseByteLimit` parser, the `sizeKnown` guard, and the
honest `&FilterSizeUnknown{}` construction). Context: spec `../spec.md`
(Implementation Decisions, Testing Decisions), ADRs `0019`, `0021`,
`0024`.
