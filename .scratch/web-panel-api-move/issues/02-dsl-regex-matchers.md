# 02: `path regex` / `agent regex` matchers in the Filter DSL

**What to build:** A rules author writes `path regex "\.(php|asp)$"` or
`agent regex "(?i)curl"` in a Filter expression and the Filter matches
Events accordingly. Patterns are unanchored searches over the Event's path
or user agent, case-sensitive by default with Go's `(?i)` as the opt-in -
consistent with the keyword matchers beside them. A bad pattern (e.g. an
unclosed group) is rejected when the rules are parsed, so a rule can never
be silently dead at runtime. The regex matcher exists on path and agent
only; method and src keep their existing matchers. The DSL's existing
string-literal escaping applies to the pattern.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `path regex "..."` parses as part of larger Filter expressions (and/or/not, alongside existing matchers).
- [x] `agent regex "..."` likewise.
- [x] Matching is unanchored: a pattern matching a substring of the value accepts the Event.
- [x] Matching is case-sensitive by default; `(?i)` in the pattern makes it case-insensitive.
- [x] An invalid pattern fails parsing with a clear error; it is never a runtime no-match.
- [x] The glossary's Filter entry records the regex-matcher semantics (unanchored search, case-sensitive unless `(?i)`).

## Answer

Implemented on `ticket/wam-02-dsl-regex-matchers` (commit `874e8a4`),
merged into `integration/web-panel-api-move` (`d77023a`).
`path regex "..."` and `agent regex "..."` parse inside larger
expressions (and/or/not, beside the existing matchers) via the
`takeMod` extension points; patterns are pre-compiled
(`FilterPathRegex`/`FilterAgentRegex` hold a `*regexp.Regexp`), so an
invalid pattern is a positioned parse error at the pattern token and a
rules load never sees a dead rule. Matching is an unanchored
`MatchString` over the Event's path/agent - case-sensitive unless the
pattern carries `(?i)`, consistent with the keyword matchers. The DSL's
string escapes apply (`\\.` for `\.`). `method`/`src` untouched. The
GLOSSARY Filter entry records the semantics, and `docs/rules.md`
documents the matchers plus the DSL-vs-panel-filter independence the
spec demands. Reviewed in the fix pass `62e3efa`.
