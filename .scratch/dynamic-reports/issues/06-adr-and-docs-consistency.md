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

**Status:** resolved

- [x] The lifecycle ADR exists, records the settled decisions and the
      three amendments to earlier ADRs, and is cross-referenced from
      the amended ones.
- [x] The glossary carries the four new terms and the reworded
      Report/Trigger entries; no implementation detail leaks into it.
- [x] Rules doc and report contract doc match shipped behavior:
      optional `ttl`, `grace`, death line, downgrade, dynamic entry
      shape, scalar replay volume, load refusals.
- [x] No stale references to volume methods anywhere in docs,
      glossary, or config examples.

## Comments

Implemented on branch `ticket/06-adr-and-docs-consistency` (lifecycle
ADR 0031 and the amendments to ADR 0002/0008/0013 in `f1d956b`,
glossary in `6dc1c65`, rules/report docs in `6eb1a2d`), targeting
integration branch `integration/dynamic-reports`.

The Trigger entry's rewording also records two shipped clauses beyond
the liveness phrase: a Trigger never Downgrades (the 0013 narrowing)
and the report adopts the overriding rung's lifecycle (02/04's Model
M) - checklist-silent, taken from the shipped behavior footers. The
`margin > volume` edge (ticket 03 flagged it as undocumented) is on
record in the rules doc and the ADR: a negative death line means the
report never dies by decay, and load does not refuse it. The Downgrade
wording follows ticket 04's shipped interpretation - the Sample and
the reading's value keep the climb's, only the recorded volume
restates - not the spec's "Sample ... move" clause. The "no downgrade
and no rewrite" quotes in the rules and report docs now read "no
downgrade from Triggers and no rewrite"; ADR 0013 keeps the quoted
phrase as the record of the narrowing itself. `docs/storage.md`'s
"reports survive restarts" claim narrows to fixed reports (the same
drift class as the 0002 amendment) - checklist-silent. The pinned
canonical example and the plan fences are untouched; the volume-method
removal notices (rules doc, ADR 0030) are deliberate, not stale.

Context: spec `../spec.md` (Implementation Decisions - Two lifecycle
modes, Report action grammar, Downgrade, Vocabulary; Further Notes -
Grace versus margin, Restart amnesty).
