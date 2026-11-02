# 02: `size unknown` selector end to end

**What to build:** A rules author writes `size unknown` - and
`not size unknown` - to handle warm-up traffic apart from sized traffic.
The form selects exactly the events whose Filesize is unknown, closing
the partition ticket 01 opens: comparisons never claim unknowns,
`size >= 0` is "the size is known", `size <= 0` matches nothing. Bare
word slips like `size known` abort load naming `unknown`, and the docs
state the degenerate limits and the size-0 contract (size 0 means
unknown; empty content is indistinguishable from not-yet-known).

**Blocked by:** 01 (shares the `size` dispatch and completes its contract)

**Status:** ready-for-agent

- [x] `size unknown` matches exactly the events whose Filesize is unknown; `not size unknown` is its complement
- [x] The partition is pinned by tests: comparisons never claim unknowns, `size >= 0` matches every known size, `size <= 0` matches nothing
- [x] `size unknown` composes with `and`/`or`/`not` and parentheses like every predicate
- [x] `size known` and other bare-word slips abort load with an error naming `unknown`
- [x] No equality operator and no `size known` synonym exist - the complement is `not size unknown`
- [x] Rules docs state the degenerate limits and the size-0 contract per ADR 0024
- [x] Tests at the existing Filter seam (Parse -> Match tables)

Context: spec `../spec.md`, ADR `0024` (size 0 means unknown).
