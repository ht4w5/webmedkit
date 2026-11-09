# 01: Remove the volume methods - replay volume is a scalar

**What to build:** A rules author writes a plain scalar `volume` on a
replay tracker, in download equivalents (the ladder `at` quantity
language is unchanged). Any volume-method block (`constant`,
`clamped-linear`, `inverse-square`, `step`) fails `bandage -t` with an
error naming the removal. The tracker reading's `volume` becomes
constant per rule, making death lines well-defined for the dynamic
lifecycle that follows. The rules doc's volume-method sections and the
glossary's Volume method entry are deleted; an ADR records the
removal. Land as its own commit in the change.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A replay tracker declares its volume as a scalar in download
      equivalents; the shipped example needs no edit.
- [x] Load refuses a volume-method block anywhere in a rules file, the
      error naming the removal (a breaking rules-format change,
      accepted pre-release).
- [x] The per-event volume re-derivation in evaluation is gone; the
      reading's volume is constant per rule.
- [x] Rules doc volume-method sections and the glossary's Volume
      method entry are deleted.
- [x] An ADR records the removal and its rationale.
- [x] All existing tests pass; method-specific tests are deleted and
      scalar replay behavior is covered instead.

## Comments

Implemented on branch `ticket/01-remove-volume-methods` (removal in
`a641e5c`, docs/ADR 0030 in `3e2d2a6`), targeting integration branch
`integration/dynamic-reports`. The ladder `at` quantity language is
unchanged; `escalations[].at: {method: ...}` keeps its existing
`must be a quantity` refusal (it is ladder quantity language, not a
volume-method block).
Context: spec `../spec.md` (Implementation Decisions - Volume-method
removal; Further Notes - work order).
