# 03: The program name stays slug-shaped

**What to build:** A guard test at the command's testable run seam asserts the
program name matches the single-label slug pattern, so a rename that produces
a malformed name fails the test suite instead of yielding non-conforming
reasons at runtime. The constraint is documented as a producer-side
requirement (ADR 0017): the report contract constrains each label's shape, so
the name the prefix is built from must satisfy it. No startup validation of a
compile-time constant — the test is the guard.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A test at the command seam asserts the program name matches
      `^[a-z0-9][a-z0-9_-]*$`
- [ ] The failure message names the constraint and points at the report
      contract's label shape, so a rename author knows why
- [ ] This is the first test on the command's pre-designed run seam; the
      command's behavior is otherwise untouched
