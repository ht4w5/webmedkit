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

**Status:** resolved

- [x] The full suite is green with zero production-code changes
- [x] Every rules document in the tests carries a chain named `main`
- [x] Nothing changed except chain names and the assertions that quote them

## Answer

Implemented on integration branch `feat/main-chain-entry` (commit
`b3a65cb`, merge `65a4364`; cross-ticket code-review fixes `4849142` +
`5d8c1d3`, merge `8f10038`). Behaviour-neutral test-suite migration:
every rules document in the test suites carries a chain named `main` —
single-chain documents rename that chain, multi-chain documents rename
their first-declared chain (file order is execution order at this
point, so every expectation is preserved). Covers the YAML fixtures in
`*_test.go`, the bench fixtures, and the Go-built chain fixtures in
`domain/evaluation`. Assertions that quote a chain name move with the
rename: the report chain field (e2e provenance), the `--test` plan
`chain ...` lines, and the duplicate-chain-name rejection fixtures
(name both chains `main`, still duplicating). Zero production-code
changes; the full suite is green (`make test`).

Context: spec `../spec.md`, `bandage/internal/app/*_test.go`,
`bandage/internal/domain/evaluation/evaluation_test.go`.

Notes:
- `docs/reports.md`'s canonical example moved too: its `"chain"` value
  rides the golden document `TestE2EReportsDocumentMatchesDocsExample`
  pins byte-exact, so it carries its fixture's chain name — a
  one-token rename (`"download-abuse"` → `"main"`), not prose.
- Later refusal fixtures (ticket 03's loader table) deliberately omit
  `main` to test rejection; every document the suites accept carries
  it.
