# 01: Namespaced reasons on reports, end to end

**What to build:** A report written by a triggered report action carries the
authored code namespaced under the daemon's program name — `bandage.<code>` —
composed from the build identity the daemon already receives, at the moment
the report is created. The namespaced form is what the report table stores and
what `GET /reports` serves. An escalation override keeps its in-place
semantics and carries the overriding rung's namespaced code. The report
document's canonical example is updated and the served response reproduces it
byte for byte. A daemon driven under a different program name serves reasons
under that name, pinning that the namespace follows the program name. Every
existing assertion on a reason string is updated in this ticket so the suite
lands green.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A created report's reason is `<program name>.<code>`; the report action
      keeps carrying the bare authored code
- [ ] The prefix is composed from the build identity at report creation only —
      not at rules load, not at serialization — as a plain join (no
      re-prefixing)
- [ ] A daemon driven under another program name serves reasons under that
      name (ADR 0017's derivation, pinned behaviorally)
- [ ] An escalation override writes the overriding rung's namespaced code in
      place; identity, code sharing across rules, and all other fields are
      unchanged
- [ ] `GET /reports` serves the namespaced form; the response reproduces the
      canonical example of the report document contract byte for byte, the
      example showing a namespaced reason
- [ ] Persistence round-trips the namespaced form unchanged; no migration or
      read-path normalization
- [ ] Rules loading is untouched: bare codes pass, dotted codes are refused
- [ ] Every existing suite assertion on a reason string is updated here; the
      full suite is green

Prior art for the tests: the end-to-end suite's report assertions through the
served report document, the report-document byte-pin test, and the rules
suite's canary build identity (for the derivation case).
