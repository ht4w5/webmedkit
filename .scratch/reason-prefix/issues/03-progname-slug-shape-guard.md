# 03: The program name stays slug-shaped

**What to build:** A guard test at the command's testable run seam asserts the
program name matches the single-label slug pattern, so a rename that produces
a malformed name fails the test suite instead of yielding non-conforming
reasons at runtime. The constraint is documented as a producer-side
requirement (ADR 0017): the report contract constrains each label's shape, so
the name the prefix is built from must satisfy it. No startup validation of a
compile-time constant — the test is the guard.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A test at the command seam asserts the program name matches
      `^[a-z0-9][a-z0-9_-]*$`
- [x] The failure message names the constraint and points at the report
      contract's label shape, so a rename author knows why
- [x] This is the first test on the command's pre-designed run seam; the
      command's behavior is otherwise untouched

## Answer

Implemented on integration branch `feat/reason-prefix` (commit
`27c563a`, merge `7d1b2f2`; cross-ticket code-review fix `d730581`,
merge `b0eb730`). `TestProgNameIsSlugShaped`, the first test at the
command's testable run seam (`bandage/cmd/bandage/main_test.go`,
package `main`, the only place the constant is visible), asserts
`progName` matches `^[a-z0-9][a-z0-9_-]*$`; its failure message names
the constraint and points at the report contract's label shape, ending
with the guide to fix the rename, not the test. No startup validation
of the compile-time constant -- the test is the guard; the command's
behavior is untouched.

Context: spec `../spec.md`, `bandage/cmd/bandage/main.go`,
`bandage/cmd/bandage/main_test.go`, ADR
`docs/adr/0017-reason-prefix-comes-from-the-progname.md`.

Notes:
- The guard asserts the constant directly ("the constant is visible
  only here"); `run` itself is not exercised -- the ticket scopes the
  seam to the guard (review disposition O4).
