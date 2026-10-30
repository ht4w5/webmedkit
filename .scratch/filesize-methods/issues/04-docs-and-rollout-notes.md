# 04: Docs and rollout notes

**What to build:** Every documentation surface matches shipped
behaviour. The log-format contract carries both response-size fields
and the rollout warning: deploy the updated nginx log format before
upgrading bandage, or the estimate stays unknown (replay blind) until
the emitter is upgraded and paths are re-observed. The replay docs
state that Filesize comes from response sizes (Content-Length,
Content-Range totals), not transferred bytes. The glossary and ADR 0019
read true against the merged code.

**Blocked by:** 02, 03

**Status:** ready-for-agent

- [ ] The log-format docs carry the `clen` and `crange` contracts and the rollout-ordering warning
- [ ] The rules/replay docs state the Filesize source: response sizes, not transferred bytes
- [ ] Glossary entries (Filesize, Filter, Sampling rule) match shipped behaviour; prose calls the estimate's inputs observations and reserves "Sample" for the report concept
- [ ] ADR 0019 reads true against merged code (Sampling rule, filter gate, lenient `crange`, no probing, no fallback)
- [ ] No documentation still describes the estimate as learned from transferred bytes

Context: spec `../spec.md` (Further Notes; Out of Scope).
