# 06: Lifecycle ADR and docs consistency

**What to build:** The decisions land where a future reader looks: an
ADR recording the dynamic-report lifecycle - amending the
trigger-semantics, durable-reports and escalation-ladder ADRs (the
ladder ADR's no-downgrade clause narrows to Triggers; the durable-
reports ADR's "reports must survive restarts" narrows to fixed
reports, restart amnesty consequence on record) - including the
grace-versus-margin note (algebraically one quantity under linear
decay; the trickle-hover band accepted). The glossary carries the new
terms - Dynamic report, Grace, Downgrade, Underflow - with Report and
Trigger reworded ("alive until expiry, Underflow of its recorded
volume, or deletion"). A final consistency pass cross-checks the
rules doc, the report contract doc and the glossary against shipped
behavior.

**Blocked by:** 01, 02, 03, 04, 05

**Status:** ready-for-agent

- [ ] The lifecycle ADR exists, records the settled decisions and the
      three amendments to earlier ADRs, and is cross-referenced from
      the amended ones.
- [ ] The glossary carries the four new terms and the reworded
      Report/Trigger entries; no implementation detail leaks into it.
- [ ] Rules doc and report contract doc match shipped behavior:
      optional `ttl`, `grace`, death line, downgrade, dynamic entry
      shape, scalar replay volume, load refusals.
- [ ] No stale references to volume methods anywhere in docs,
      glossary, or config examples.
