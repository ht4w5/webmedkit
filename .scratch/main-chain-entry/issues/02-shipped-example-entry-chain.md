# 02: Shipped example rules carry an entry chain

**What to build:** The shipped example rules document gets a `main`
dispatcher chain — one filter-less `jump` rule per topical chain — so it
has a front door and nothing in it is dead once entry-chain evaluation
lands. The dispatcher sequences exactly like today's file order (a
`jump` is non-terminal), and the guide's worked-example excerpts stay
verbatim in the shipped file (the two are coupled by their test).
`bandage -t` accepts both shipped documents. Under today's semantics
this is behaviour-neutral; it is the config-side prerequisite for the
evaluation flip.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The shipped example loads and validates with a `main` chain dispatching to every topical chain
- [x] The guide/shipped-example coupling tests pass unmodified
- [x] `bandage -t` accepts both shipped documents

## Answer

Implemented on integration branch `feat/main-chain-entry` (commit
`70bb6a8`, merge `c7d680b`; cross-ticket code-review fixes `4849142` +
`5d8c1d3`, merge `8f10038`). The shipped example `configs/rules.yml`
gains a `main` dispatcher chain: one filter-less `jump` rule per
topical chain, in file order (a `jump` is non-terminal, so the
dispatcher sequences exactly like the chains' own file order) — pinned
at the loader seam by `TestShippedRulesMainDispatchesEveryTopicalChain`
(alongside `TestShippedRulesLoad`). The guide's worked-example excerpts
stay verbatim in the shipped file
(`TestShippedRulesCarryTheGuideLadder`,
`TestShippedRulesCarryTheGuideDispatcher` pass unmodified — ticket 02
only appended its own test), and `bandage -t` accepts both shipped
documents (verified with the binary against `configs/bandage.yml` +
`configs/rules.yml`; the site pair validates too). Config-side only:
behaviour-neutral under the pre-flip semantics.

Context: spec `../spec.md`, `configs/rules.yml`,
`bandage/internal/app/shippedrules_test.go`, `docs/rules.md` (coupled
excerpts), ADR `docs/adr/0015-main-chain-entry.md`.

Notes:
- The dispatcher covers `download-abuse`, `api` and `iso-egress` and
  deliberately NOT `inspect` — `inspect` is the ladder's handoff
  target (entered by its `jump` rung alone), so dispatching it from
  `main` as well would double-feed its replay tracker on handoff
  events. Ticket 04's comments depend on this.
