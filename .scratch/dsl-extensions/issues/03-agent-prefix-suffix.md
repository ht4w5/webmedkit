# 03: Agent prefix/suffix matchers end to end

**What to build:** A rules author writes `agent prefix "curl/"` and
`agent suffix ".0"` and matches user agents by their start or end
without regexes: literal, case-sensitive, with the exact semantics of
`path prefix|suffix`. Empty text aborts load, the existing `agent`
exact/keyword/regex forms are untouched, and the docs list the new
forms.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `agent prefix "text"` matches agents starting with text; `agent suffix "text"` matches agents ending with it
- [x] Both match literally and case-sensitive, like `path prefix|suffix`
- [x] Empty text aborts load (prefix/suffix must not be empty), like every other text matcher
- [x] Exact/keyword/regex agent forms unchanged; existing rules files parse identically
- [x] Rules docs list the new forms
- [x] Tests at the existing Filter seam (Parse -> Match tables), mirroring the `path prefix|suffix` tables

Context: spec `../spec.md`.

Implemented on integration branch `integration/dsl-extensions` (merge
commit `37a8968`; cross-ticket code-review fixes through `5af6d25`,
incl. the shared `parseTextMod` behind `path` and `agent` text
modifiers). Context: spec `../spec.md` (Implementation Decisions,
Testing Decisions).
