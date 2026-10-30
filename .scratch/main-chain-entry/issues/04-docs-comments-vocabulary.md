# 04: Docs, comments and vocabulary

**What to build:** The operator-facing text tells the truth about the
new model. The rules guide explains entry-chain semantics, the
dispatcher pattern the shipped example uses, and the double-dispatch
pitfall; the project plan's pipeline, actions and validation text is
updated to match. The shipped site config's "every chain runs on every
event" comment is corrected: replay inspection runs only on handoff and
takes each handoff event once. The glossary's Entry chain and Chain
terms and ADR 0015 are checked against the shipped behaviour. The
guide's worked-example excerpts stay verbatim-coupled to the shipped
example.

**Blocked by:** 03

**Status:** resolved

- [x] Rules guide, project plan and config comments describe main-chain entry and the pitfall
- [x] Glossary and ADR 0015 match the shipped behaviour
- [x] The guide/shipped-example coupling tests stay green

## Answer

Implemented on integration branch `feat/main-chain-entry` (commit
`f5a0df5`, merge `7c5c795`; cross-ticket code-review fixes `4849142` +
`5d8c1d3`, merge `8f10038`). The operator-facing text tells the truth
about the new model: `docs/rules.md` gains "Entry chains and dispatch"
(entry-chain semantics, the dispatcher pattern the shipped example
uses, the double-dispatch pitfall) with its dispatcher and ladder
excerpts verbatim-coupled to `configs/rules.yml` by
`TestShippedRulesCarryTheGuideDispatcher` /
`TestShippedRulesCarryTheGuideLadder`; `PLAN.md`'s pipeline, actions
and validation wording describes main-chain entry; the shipped site
config's "every chain runs on every event" comment is corrected -
replay inspection runs only on handoff and takes each handoff event
once. The glossary's *Entry chain* and *Chain* terms (plus *Dispatch*)
and ADR 0015 are checked against the shipped behaviour; the coupling
tests stay green.

Context: spec `../spec.md`, `docs/rules.md`, `PLAN.md`, `GLOSSARY.md`,
`configs/site/rules.yml`, ADR `docs/adr/0015-main-chain-entry.md`.

Notes:
- The `configs/site/rules.yml` comment correction is on disk only: that
  path is git-ignored on purpose (site-specific data, `.gitignore`
  `/configs/site`), so the checkbox holds without a commit.
- The review fix pass (`4849142`) added the GLOSSARY `Dispatch` entry
  (vocabulary gap it found) and de-duplicated the dispatcher rationale
  (it now lives in `docs/rules.md`; the shipped-file and test comments
  point at it).
