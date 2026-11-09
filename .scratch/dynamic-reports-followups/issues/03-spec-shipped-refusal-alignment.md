# 03: The dynamic-reports spec matches the shipped refusal

**What to build:** The dynamic-reports spec still promises "an error
naming the removal" for a volume-method block (story 28, the Load
validation additions bullet, the Volume-method removal decision) -
wording the no-checks-for-removed-syntax review decision superseded.
The three lines name the shipped refusal: the ordinary quantity
validation fails the block at `bandage -t`, with no dedicated check
kept for removed syntax. Nothing else in the spec moves.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Story 28, the Load validation additions bullet and the
      Volume-method removal decision match shipped behavior
- [x] No other spec text changes

## Comments

Implemented on integration branch `integration/dynamic-reports`
(commit `c2d8b13`); the shipped refusal itself landed in `b2a31cf`
(no checks for removed rule syntax). The three lines now say the
ordinary quantity validation fails the block at `bandage -t`.
Context: spec `../spec.md`; the record of the decision is
`.scratch/dynamic-reports/spec.md` (Volume-method removal) and
`docs/adr/0030-replay-volume-is-a-plain-scalar.md`.
