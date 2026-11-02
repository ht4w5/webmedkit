# 01: Single-quoted strings end to end

**What to build:** A rules author writes any Filter text value in
single quotes - `path regex '\.iso$'` behaves exactly like `path regex
"\\.iso$"` - with `''` as the only escape and every other character,
backslashes included, literal. The spelling works at every text value
(`path`/`agent` exact and the `prefix|suffix|keyword|regex`
modifiers), an unterminated single-quoted string aborts `bandage -t`
with the DSL's position-carrying error at the opening quote, and the
rules docs teach both spellings with their escape rules and show the
regex examples single-quoted.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Single-quoted strings accepted at every text value, interchangeable with double-quoted ones; the pattern reaching the regex engine is byte-identical to the quoted text modulo `''`
- [ ] `''` is the only escape and yields one literal quote; backslashes are inert; the string ends at the first unpaired quote (`'foo\'` is `foo\`, `''''` is two quotes)
- [ ] Single-quoted strings span newlines like double-quoted ones
- [ ] An unterminated single-quoted string aborts load with `unterminated string` at the opening quote's 1-based position; `unknown escape sequence` stays exclusive to the double-quoted form (pinned by contrast tests)
- [ ] Empty `''` is rejected at `prefix`/`suffix` exactly like `""`
- [ ] The quote character terminates bare words (parity with the double quote); every rules file that loads today parses identically
- [ ] Single-quoted values compose with `and`/`or`/`not` and parentheses like every predicate; numbers, size units, and CIDRs are unaffected
- [ ] Regex matcher semantics unchanged (unanchored, case-sensitive unless `(?i)`)
- [ ] Rules-file format version stays 0; a rules file using single-quoted strings fails loudly on an older binary at load
- [ ] Rules docs list both spellings and their escape rules, recommend single quotes for regex patterns, and show the examples in that spelling
- [ ] Parse errors keep the 1-based line/column contract and validation errors wrap unwrappably
- [ ] Tests at the existing Filter seam (Parse -> Match tables): equivalence pairs, `''` and backslash-literal match cases, parse-tree parity, parse-error positions, and the contrast pins

Context: spec `../spec.md`, plan `../plan.md` (superseded design
record; the grilling decisions it lists are binding).

## Comments

- 2026-11-02: Code-review adjudication - bare-word terminator vs
  story 15. The quote character stays a bare-word terminator (parity
  with `"`; grilling-settled Implementation Decision, normative), even
  though story 15 ("every rules file that loads today parses
  identically") is literally unsatisfiable alongside the feature: a
  token-start `'` must lex as a string, so e.g. `method 'GET'` cannot
  keep lexing as the bare word `'GET'`. Parity kept. Residual breakage
  is bounded to method values containing a quote character - the one
  input class that loads today and lexes differently after - and is a
  loud load-time failure, never a silent semantic change (story 16's
  principle preserved): `method don't` fails with
  `filter: L:C: unterminated string` at the quote, `method 'GET'`
  fails with the expected-bare-method-name error. Spec Implementation
  Decisions corrected accordingly (bare words can also be method
  names); story 15's wording and the terminator decision itself are
  unchanged.
- 2026-11-02: Code-review correction - `''''` spelling. Per story 3
  (`''` yields one literal quote) and the first-unpaired-quote rule,
  `''''` is ONE literal quote (`''''''` is two); the code and its
  assertion were already correct. The spec example, the docs sentence,
  and the test row name that said "two quotes" are fixed to match.
