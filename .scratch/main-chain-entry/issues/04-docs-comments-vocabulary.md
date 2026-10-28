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

**Status:** ready-for-agent

- [ ] Rules guide, project plan and config comments describe main-chain entry and the pitfall
- [ ] Glossary and ADR 0015 match the shipped behaviour
- [ ] The guide/shipped-example coupling tests stay green
