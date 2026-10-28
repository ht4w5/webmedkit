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

**Status:** ready-for-agent

- [ ] The shipped example loads and validates with a `main` chain dispatching to every topical chain
- [ ] The guide/shipped-example coupling tests pass unmodified
- [ ] `bandage -t` accepts both shipped documents
