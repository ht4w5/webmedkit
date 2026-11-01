# 07: docs/rules.md vocabulary sweep

**What to build:** (a) `docs/rules.md:94-95` "one pipeline with a single
front door" -> "one entry chain with a single front door" (Chain
_Avoid_: pipeline). (b) Sweep "stands above" to "exceeds" everywhere,
keeping "crosses" as the event word: `docs/rules.md:24, 282-284, 414`
("the actions of every rung the value stands above" -> "exceeds",
"when the value stands above a rung", etc.) and the comment at
`bandage/internal/app/rules.go:395` ("the rungs a value stands above
stay a ladder prefix" -> "exceeds"). GLOSSARY.md is already done.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Comments

Context: resolved on branch `ticket/dc-07-rules-vocabulary-sweep` (spec
`../spec.md`, Overflow vocabulary). `docs/rules.md` now says "one entry
chain with a single front door" and uses "exceeds" for the state
throughout ("crosses" kept for the event) - the parseEscalations
comment at `bandage/internal/app/rules.go:395` swept too. Also changed
"stays above" -> "exceeds" at rules.md:288 (same state vocabulary,
matches the glossary's already-applied phrasing). GLOSSARY.md untouched
(already done). `make test` green (the doc-pinning suites pass).
