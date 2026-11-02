# 03: Agent prefix/suffix matchers end to end

**What to build:** A rules author writes `agent prefix "curl/"` and
`agent suffix ".0"` and matches user agents by their start or end
without regexes: literal, case-sensitive, with the exact semantics of
`path prefix|suffix`. Empty text aborts load, the existing `agent`
exact/keyword/regex forms are untouched, and the docs list the new
forms.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `agent prefix "text"` matches agents starting with text; `agent suffix "text"` matches agents ending with it
- [ ] Both match literally and case-sensitive, like `path prefix|suffix`
- [ ] Empty text aborts load (prefix/suffix must not be empty), like every other text matcher
- [ ] Exact/keyword/regex agent forms unchanged; existing rules files parse identically
- [ ] Rules docs list the new forms
- [ ] Tests at the existing Filter seam (Parse -> Match tables), mirroring the `path prefix|suffix` tables

Context: spec `../spec.md`.
