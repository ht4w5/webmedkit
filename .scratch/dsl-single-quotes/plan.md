# Filter DSL single-quoted strings - plan

**Status:** superseded by `spec.md` in this directory (the grilling
session settled the naming as "single-quoted strings", multi-line
parity, the shared unterminated-string error, in-change docs
migration, all-text-value scope, and the quote character as a
bare-word terminator - see `spec.md`, which is authoritative). Kept as
the design-session record.

**Base:** on top of `main` (the Filter DSL it extends is `dsl-extensions`,
merged at `b0df22b`).

## Motivation

From the `configs/site/rules.yml` review: regex patterns need literal
backslashes (`\.`, `\.`), but the double-quoted string form processes
escapes, so every backslash must be doubled (`"repomd\\.xml(\\.(asc|key))?$"`)
- noisy, error-prone, and a fresh failure mode for rules authors
(`unknown escape sequence \.` at load). A single-quoted spelling where
backslashes are inert (`'repomd\.xml(\.(asc|key))?$'`) writes the pattern
the way the regex engine sees it.

## Requirements (what this plan must deliver)

1. A rules author can write any text value as a single-quoted string
   `'...'` in which the only escape is `''` for one literal quote;
   every other character - backslashes included - is literal.
2. Single- and double-quoted strings are interchangeable wherever the
   Filter grammar takes a text value: `path`/`agent` exact and the
   `prefix|suffix|keyword|regex` modifiers.
3. `path regex '\.iso$'` works with no doubling; the pattern handed to
   `regexp.Compile` is byte-identical to the quoted text modulo `''`.
4. Malformed raw strings (unterminated) abort load with the DSL's
   position-carrying errors; empty text is still rejected by the
   validating text-match constructors (`prefix`/`suffix`).
5. No rules file that loads today changes meaning; a file using `'...'`
   fails loudly on an older binary.

## Current state (what moves where)

| Concern | Today | After |
|---|---|---|
| String lexing | `token.go scanString`: `"..."` with escapes `\"`, `\\`, `\n`, `\t`; any other `\X` errors `unknown escape sequence \X` | + `scanRawString` for `'...'`; `scanString` untouched |
| Word boundaries | `isWordRune` excludes `( ) " < > ! | &` + whitespace; `'` is a word rune, so `'foo'` lexes as one bare word `'foo'` | `'` terminates bare words, parity with `"` |
| Text values | `parse.go takeText` consumes `tokString` at one call site (the text-modifier path shared by `path` and `agent`) | unchanged - both spellings yield `tokString` |
| Parser | `parseTextMod` + the text-match constructor set | unchanged |
| Error contract | `ParseError{Line,Col,Msg,err}`, 1-based; constructor errors wrap unwrappably | unchanged; unterminated raw string reuses the `unterminated string` message at the opening quote |
| Docs | `docs/rules.md` Filter section | + a string-literals note |

## Target design

### Grammar

```
string = '"' ... '"'    (* existing: escapes \" \\ \n \t *)
       | "'" ... "'"    (* raw: only '' is an escape *)
```

- Inside `'...'`: `''` yields one `'`; a backslash is always literal;
  the string ends at the first unpaired `'` (so `'foo\'` is `foo\`).
- Both forms produce the same `tokString` token; `takeText` and every
  consumer stay untouched - one seam, no parser fork.
- `''` is a valid empty text token; the text-match constructors reject
  empty `prefix`/`suffix` text exactly as they reject `""` today.

### Lexer changes (`token.go`)

- `next()`: `case '\''` dispatches `scanRawString(line, col)`.
- `scanRawString`: scan to the closing quote; `''` appends one `'`;
  unterminated -> `errAt(openLine, openCol, "unterminated string")` -
  same message and position shape as `scanString`.
- `isWordRune`: `'` joins `"` in the terminator set.

### Error contract

- Unterminated raw string: `filter: L:C: unterminated string` at the
  opening quote (parity with `"`).
- `unknown escape sequence` stays exclusive to `"..."` - raw strings
  cannot raise it. The contrast is the feature and gets a test.
- 1-based line/column and unwrappable validation errors unchanged.

### Compatibility

- Every rules file that loads today parses identically: a bare word with
  `'` in it cannot appear in a successfully-loading file (words are
  keywords, numbers, or CIDRs), so the lexing change can only alter
  inputs that previously failed to load - which now gain a spelling.
- Format version stays 0: the addition is additive, and an older binary
  rejects `'...'` at load (`expected a quoted text value after "path",
  got "'foo'"`) - loud, never silent under-match.
- `size` values are numbers and unaffected.

### Docs (`docs/rules.md`)

- A "string literals" note in the Filter section: the two spellings,
  their escape tables, and the recommendation to quote regex patterns
  single.
- Regex examples switch to the raw spelling where it reads better.

## Testing

Existing seam only: the Filter package's Parse -> Match tables. No new
seams; no test reaches into lexer internals.

- Match table: raw/escaped equivalence (`agent prefix 'curl/'` behaves
  as `agent prefix "curl/"`), `''` -> literal quote (e.g. `agent suffix
  'don''t'` matches an agent ending `don't`), backslash text literal.
- Parse-tree table: `path regex '\.iso$'` yields the same tree as
  `path regex "\\.iso$"`.
- Parse-error table: unterminated raw string (`path 'foo`) asserting
  `filter: 1:6: unterminated string`; contrast pin: `\.` in `"..."`
  still errors `unknown escape sequence \.`, in `'...'` it is literal.
- Constructor table: `agent prefix ''` rejected like `agent prefix ""`.
- Prior art: the `path prefix|suffix` empty-text rows and the existing
  `scanString` escape tests.

## Out of Scope

- New escapes in `"..."` (`\'`, `\u`) or changing the existing escape set.
- Interpolation, concatenation, heredocs.
- YAML layer (block scalars are already literal).
- Web Panel report filter (client-side search over Reports; separate
  vocabulary per GLOSSARY).
- Anything outside text values: numbers, CIDRs, size units.

## Open questions (for the grilling session)

1. Docs name: "raw strings" or "single-quoted strings"? (Whatever wins
   should match GLOSSARY style; the other becomes avoid-vocabulary.)
2. Multi-line: `"..."` may span newlines today; raw strings would too by
   parity. Keep parity or forbid newlines in raw strings?
3. Error wording: share `unterminated string` with `"..."`, or say
   `unterminated single-quoted string` for a clearer load failure?
4. Should the docs migration of existing regex examples happen in this
   change or a follow-up docs pass?
