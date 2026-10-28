# 01: Prefactor — test rules documents name their entry chain

**What to build:** No behaviour change, no production code touched.
Every rules document in the test suites declares its front door
explicitly: a document with one chain names that chain `main`, and a
document with several chains names its first-declared chain `main` (file
order is execution order today, so every expectation is preserved).
Assertions that quote a chain name — for example a report's chain field —
move with the rename. This is the mechanical batch that makes the
semantics flip easy before the easy change.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The full suite is green with zero production-code changes
- [ ] Every rules document in the tests carries a chain named `main`
- [ ] Nothing changed except chain names and the assertions that quote them
