# Spec: Single-quoted strings in the Filter DSL

Status: ready-for-agent

## Problem Statement

The Filter DSL's only string spelling double-quoted, processes escapes
(`\"`, `\\`, `\n`, `\t`), so any value containing backslashes must be
doubled to survive lexing: a regex like `repomd\.xml(\.(asc|key))?$`
must be written `"repomd\\.xml(\\.(asc|key))?$"`. Rules authors write
patterns nothing like the pattern the regex engine sees, every single
backslash is a potential load failure (`unknown escape sequence \.`),
and the doubling error surfaces far from the author's intent. A real
rules file review (`configs/site/rules.yml`) hit exactly this: five
regex values failed to load, all of them escapes.

## Solution

The Filter DSL gains single-quoted strings `'...'` in which the only
escape is `''` for one literal quote; every other character -
backslashes included - is literal. They are accepted at every text
value and are interchangeable with double-quoted strings, so a pattern
reads the way it matches: `path regex '\.iso$'`. The double-quoted
spelling and its escapes are unchanged.

## User Stories

1. As a rules author, I want to write regex patterns in single quotes
   with backslashes untouched, so that the pattern reads exactly as the
   regex engine sees it.
2. As a rules author, I want `''` to be the only escape inside
   single-quoted strings, so that nothing else in my text can trip an
   escape rule.
3. As a rules author, I want `''` to yield one literal quote, so that
   user agents and paths containing apostrophes stay matchable.
4. As a rules author, I want single- and double-quoted strings
   interchangeable at every text value (`path`/`agent` exact and the
   `prefix|suffix|keyword|regex` modifiers), so that quoting is a
   per-value choice, not a per-keyword one.
5. As a rules author, I want backslashes literal in every single-quoted
   value, not just regex patterns, so that literal text carrying
   backslashes needs no doubling either.
6. As a rules author, I want the pattern handed to the regex engine
   byte-identical to my quoted text (modulo `''`), so that what I write
   is what matches.
7. As a rules author, I want `path regex '\.iso$'` to match exactly
   like `path regex "\\.iso$"`, so that the two spellings are honest
   alternatives with one meaning.
8. As a rules author, I want an unterminated single-quoted string to
   abort load with the DSL's position-carrying error at the opening
   quote, so that mistakes surface in `bandage -t`, never at runtime.
9. As a rules author, I want empty single-quoted text rejected at
   `prefix`/`suffix` exactly like `""`, so that a matcher that can
   never miss is a configuration error under either spelling.
10. As a rules author, I want the double-quoted escape contract
    unchanged, so that existing rules and habits keep their meaning -
    and a `\.` in `"..."` still fails loudly.
11. As a rules author, I want single-quoted strings to span newlines
    like double-quoted ones, so that multi-line filter blocks behave
    the same whichever spelling I pick.
12. As a rules author, I want numbers, size units, and CIDRs untouched
    by the change, so that every non-text value parses exactly as
    before.
13. As a rules author, I want regex matcher semantics unchanged
    (unanchored, case-sensitive unless the pattern carries `(?i)`), so
    that quoting changes spelling only, never behavior.
14. As a rules author, I want single-quoted values to compose with
    `and`, `or`, `not` and parentheses like every predicate, so that
    the new spelling costs nothing in expression structure.
15. As an operator, I want every rules file that loads today to parse
    identically, so that upgrading is a non-event.
16. As an operator, I want a rules file using single-quoted strings to
    fail loudly on an older binary at load, so that version skew is
    visible and never silently under-matches.
17. As an operator, I want the rules-file format version to stay 0, so
    that the additive vocabulary follows the recorded versioning
    decision (a bump is reserved for breaking changes).
18. As an operator, I want the rules documentation to list both
    spellings with their escape rules and show regex examples
    single-quoted, so that rules authors never have to read code to
    write filters.
19. As an operator, I want parse errors to keep the 1-based
    line/column contract and validation errors to wrap unwrappably, so
    that tooling and tests rely on one error story.
20. As an operator, I want the vocabulary addition confined to the
    Filter language, so that no Event, wire, report, or config surface
    changes.

## Implementation Decisions

- The canonical term is **single-quoted strings**; "raw string" is
  avoid-vocabulary in docs and code comments. The term is deliberately
  not added to the glossary: string spelling is syntax, not domain
  language, and no ADR records it (the decision is additive,
  unsurprising, and cheap to extend - it fails the three-part ADR
  test).
- The grammar gains a second string literal: the existing double-quoted
  form with its four escapes, and the single-quoted form where `''`
  yields one quote and every other character - including backslash -
  is literal. The string ends at the first unpaired quote, so `'foo\'`
  is `foo\` and `''''` is two quotes.
- Both spellings produce the same string token; the text-value consumer
  and the parser are unchanged - one seam, no parser fork. Empty `''`
  is a valid empty text token and is rejected where empty text is
  rejected today (the `prefix`/`suffix` constructors), exactly like
  `""`.
- The lexer gains a single-quoted scan routine mirroring the existing
  one: unterminated strings raise the same `unterminated string` error
  at the opening quote, and `unknown escape sequence` remains exclusive
  to the double-quoted form - raw-quoted text can never raise it.
- Single-quoted strings may span newlines, parity with the
  double-quoted form.
- The quote character joins the set that terminates bare words (parity
  with the double quote). This can only change inputs that fail to load
  today: a successfully-loading file's bare words are keywords,
  numbers, or CIDRs and cannot contain a quote character.
- Compatibility and versioning: the addition is additive; the
  rules-file format version stays 0, and an older binary rejects a
  single-quoted string at load (it lexes as a bare word and fails the
  text-value check) - loud failure, never silent under-match. No
  Event, wire, report, or config change.
- The rules documentation gains a string-literals note in its Filter
  section - two spellings, their escape tables, single quotes
  recommended for regex patterns - and its regex examples migrate to
  the single-quoted spelling in the same change.
- Text values are the entire scope of the spelling: `path`/`agent`
  exact and the `prefix|suffix|keyword|regex` modifiers. Numbers
  (`size`, `sent`, `status`), CIDRs, and size units are unaffected.

## Testing Decisions

- Good tests assert external behavior only: DSL source in, match
  result out against an Event; parse failures asserted through error
  text and positions. No test reaches into lexer, parser, or filter
  internals.
- One seam, an existing one: the Filter package's public surface
  (Parse -> Match), exercised by its table-driven tests - the match
  table with per-case event mutations, the parse-tree table, the
  parse-error table, and the constructor-validation table. No new
  seams.
- Prior art: the `path prefix|suffix` empty-text rows, the existing
  double-quoted escape tests, and the `size`/`agent` tables from the
  Filter DSL extensions spec in those same tests.
- New match cases: single-quoted/escaped equivalence pairs (`'curl/'`
  behaves as `"curl/"`), `''` yielding a literal quote in matched
  text, backslash-literal text, and a regex pair pinning `'\.iso$'` to
  the behavior of `"\\.iso$"`.
- New parse cases: parse trees for single-quoted values; the
  unterminated case with its `filter: L:C: unterminated string`
  position at the opening quote; the contrast pins - `\.` in `"..."`
  raises `unknown escape sequence \.`, the same text in `'...'` is
  literal; empty `''` rejected at `prefix`/`suffix` like `""`.
- The multi-line behavior rides on the existing scan routine's
  structure and needs no dedicated case beyond one parse-tree row.

## Out of Scope

- New escapes in double-quoted strings (`\'`, `\u`) or any change to
  the existing escape set.
- Interpolation, concatenation, heredocs, or any third spelling.
- YAML-layer changes (block scalars are already literal; the escapes
  live entirely in the Filter language).
- Web Panel report filter - a separate client-side search over stored
  Reports with its own vocabulary.
- Values other than text: numbers, size units, CIDRs.
- Glossary or ADR changes (deliberate; see Implementation Decisions).

## Further Notes

- This spec is the outcome of a design grilling session over
  `.scratch/dsl-single-quotes/plan.md`, which is superseded but kept
  as the design-session record. The session settled: the canonical
  name, multi-line parity, the shared unterminated-string error,
  in-change docs migration, all-text-value scope, and the quote
  character as a bare-word terminator.
- Motivated by the `configs/site/rules.yml` review, where five regex
  values failed to load on single-backslash escapes and the docs had
  to teach the doubling workaround.
- Follows the Filter DSL extensions precedent: additive vocabulary,
  format version 0, one test seam, parse errors carrying 1-based
  positions.
