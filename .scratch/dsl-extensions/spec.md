# Spec: Size predicates and agent prefix/suffix in the Filter DSL

Status: ready-for-agent

## Problem Statement

Rules authors can filter on the bytes an event actually transferred
(`sent`) but not on how big the served content *is*: there is no way to
scope a rule to large downloads, small files, or events whose Filesize
is still unknown. The estimate is right there on the Event and feeds
replay already, yet the Filter language cannot see it.

User-agent matching is similarly boxed in: only exact, `keyword`, and
`regex` forms exist, so "starts with `curl/`" or "ends with `.0`" forces
a regex - escaping rules, no literal semantics, and nothing to suggest
the intent is a plain prefix or suffix the way `path prefix|suffix`
already expresses it.

## Solution

The Filter DSL gains `size` predicates over the Filesize estimate:
`size >= 100m`, `size <= 1m`, and the explicit `size unknown`, with the
same binary size units `sent` takes. Comparisons match events of known
size only - an unknown-size event is never claimed by `size >=` or
`size <=` - and `size unknown` selects exactly those events. `agent`
gains `prefix` and `suffix` modifiers with the exact semantics of
`path prefix|suffix`: literal, case-sensitive, empty text rejected.

## User Stories

1. As a rules author, I want `size >= N` / `size <= N` predicates over
   the Filesize estimate, so that I can scope rules to how big the
   served content is.
2. As a rules author, I want size limits spelled with binary units
   (`k`, `m`, `g`) exactly like `sent`, so that one unit convention
   serves both byte quantities.
3. As a rules author, I want `size unknown` to select events whose
   Filesize is unknown, so that warm-up traffic can be handled apart
   from sized traffic.
4. As a rules author, I want `size` comparisons to skip unknown-size
   events entirely, so that "large files only" never claims traffic the
   estimate knows nothing about.
5. As a rules author, I want `size >= 0` to mean "the size is known",
   so that I can separate sized from unsized events without a special
   form.
6. As a rules author, I want `size <= 0` to be well-defined (it matches
   nothing, since a learned size is never 0), so that no degenerate
   limit surprises me at runtime.
7. As a rules author, I want `not size unknown` to express the
   complement, so that both directions of the partition are available.
8. As a rules author, I want `size` to decide over the Filesize estimate
   and `sent` over the transferred bytes, never conflated, so that "how
   big is the file" and "how much did this request send" stay separate
   questions.
9. As a rules author, I want malformed `size` forms (`size 5`,
   `size known`, a non-numeric limit) rejected at load with the DSL's
   usual position-carrying errors, so that mistakes surface in `bandage
   -t`, never at runtime.
10. As a rules author, I want size units accepted only where byte
    quantities live (`sent` and `size`), with an error naming both
    keywords, so that `status 200k`-style slips fail with a message that
    points at the right vocabulary.
11. As a rules author, I want `agent prefix "text"` and
    `agent suffix "text"`, so that user agents can be matched by their
    start or end without regexes.
12. As a rules author, I want the agent prefix/suffix forms to be
    literal and case-sensitive like `path prefix|suffix`, so that one
    DSL carries one text-matching story.
13. As a rules author, I want empty prefix/suffix text rejected at
    load, so that a matcher that can never miss is a configuration
    error, not a silent catch-all.
14. As a rules author, I want the existing `agent` exact, `keyword`, and
    `regex` forms unchanged, so that no deployed rules file changes
    meaning.
15. As an operator, I want the rules documentation to list every new
    predicate and its semantics, so that rules authors never have to
    read code to write filters.
16. As an operator, I want the `size`-in-`enrichment.filter` trap
    documented, so that nobody writes a gate that is silently `any` or
    silently `none`.
17. As an operator, I want existing rules files to keep loading exactly
    as before, so that upgrading is a non-event.
18. As an operator, I want a rules file written for the new vocabulary
    to fail loudly on an older binary (unknown keyword at load), so that
    version skew is visible and never silently under-matches.
19. As an operator, I want the "size 0 means unknown" contract recorded
    as a decision, so that future readers do not "fix" the missing
    known-empty state.
20. As an operator, I want empty content documented as indistinguishable
    from not-yet-known, so that expectations around zero-byte files are
    set at write time.
21. As a rules author, I want `size unknown` and `size` comparisons
    composable with `and`, `or`, `not` and parentheses like every other
    predicate, so that the new vocabulary costs nothing to combine.

## Implementation Decisions

- The Filter DSL gains a `size` primitive with two shapes: a comparison
  (`>=` or `<=` then a size, optional binary unit, mirroring `sent`) and
  the word form `size unknown`. There is deliberately no equality
  operator and no `size known` synonym (`not size unknown` is the
  complement).
- Predicate semantics: `size >= N` / `size <= N` match only events whose
  Filesize is known, inclusive at the boundary; the unknown marker (0)
  never matches a comparison. Consequently `size >= 0` is the "size is
  known" predicate and `size <= 0` matches nothing, since a learned size
  is never 0.
- New Filter implementations FilterSizeAtLeast, FilterSizeAtMost, and
  FilterSizeUnknown, with validating constructors (negative limits
  rejected) mirroring the `sent` filters; the unknown form takes no
  argument, like `any`/`none`.
- `agent` gains `prefix` and `suffix` modifiers backed by new Filter
  implementations with validating constructors (empty text rejected),
  matching literally and case-sensitive - exact parity with
  `path prefix|suffix`.
- The parser gains a `size` keyword dispatch handling both shapes, and
  the agent modifier set gains `prefix` and `suffix`. The unit-gate
  error message names both keywords allowed to carry size units. Parse
  errors keep the DSL's 1-based line/column contract and wrap validation
  errors unwrappably, as today.
- The rules-file format version stays 0: the vocabulary addition is
  additive, every existing file parses identically, and a too-new file
  fails loudly at load. A version bump is reserved for breaking changes.
- `size` predicates are accepted in `enrichment.filter` but documented as
  a trap: the gate runs after the derived-field reset and before the
  estimate lookup, so every event reaches it unknown - `size unknown`
  there is a no-op gate like `any`, and any `size` comparison disables
  enrichment outright. Deliberately not rejected at load; the maintainer
  chose documentation over parser policing.
- The size contract itself - size 0 means unknown, known-empty is
  unrepresentable, the Sampling rule observes only non-empty response
  sizes - is recorded in ADR 0024 and the glossary, which land with this
  spec.
- No Event, wire, or report-schema change: the `size_bytes` contract
  (ADR 0021) is untouched, and `sent` predicates are unchanged (ADR
  0019: the estimate never falls back to transferred bytes).
- The rules documentation gains the new predicates in its Filter
  section: the `size` forms with their known-size-only comparison
  semantics and degenerate-limit notes, the `agent` prefix/suffix forms,
  the unit note updated to name both unit-carrying keywords, and the
  enrichment-gate trap note.

## Testing Decisions

- Good tests assert external behavior only: DSL source in, match result
  out against an Event; parse failures asserted through error text and
  positions. No test reaches into parser or filter internals.
- One seam, an existing one: the Filter package's public surface
  (Parse -> Match), exercised by its table-driven tests - the match
  table with per-case event mutations, the parse-tree table, the
  parse-error table, and the constructor-validation table. No new seams.
- Prior art: the `sent` comparison tables (inclusive boundaries, binary
  units, zero limit) and the `path prefix|suffix` tables (literal,
  case-sensitive, empty rejection) in those same tests.
- New match cases: `size` inclusive boundaries; binary units; both
  comparisons skipping unknown-size events; `size unknown` selecting
  unknowns and skipping known sizes; `not size unknown`; the degenerate
  `size >= 0` / `size <= 0` results; agent prefix/suffix hits, misses,
  and case sensitivity.
- New parse cases: parse trees for all new forms; error text and
  positions for `size 5`, `size known`, a missing or non-numeric limit,
  size overflow, empty agent prefix/suffix text; the updated unit-gate
  message.
- The enrichment-gate trap's precondition (the derived-field reset runs
  before the gate) is already pinned at the enrichment tests, and the
  "zero-length responses are not observations" invariant is already
  pinned at the ingest and enrichment tests; neither needs new tests.

## Out of Scope

- Making "known empty" representable (sentinel rework, optional value,
  or known flag) - rejected alternative, recorded in ADR 0024.
- An equality operator on `size` or `sent`, and a `size known` synonym.
- Load-time rejection of `size` in `enrichment.filter` (documented trap
  instead, per the maintainer's decision).
- Web panel report-filter changes; the panel filter is a separate
  client-side search over stored Reports.
- Wire or report schema changes; rules-file format version bump.
- Changes to `sent` predicate semantics or vocabulary.
- Changes to the Sampling rule code - it already observes only non-empty
  response sizes; only its documented wording is sharpened.

## Further Notes

- This spec is the outcome of a design grilling session; the vocabulary
  follows the glossary (Filter, Filesize, Sampling rule) and the
  decisions in ADR 0019 (the estimate never falls back to transferred
  bytes), ADR 0021 (the sample mirrors the event), and ADR 0024
  (size 0 means unknown).
- Both features share one feature directory by maintainer decision -
  they ship as three tracer-bullet tickets (size comparisons; the
  `size unknown` selector; agent prefix/suffix) under this one spec.
- Supersedes the "filter predicates for response sizes" deferral noted
  in the Filesize-methods spec's future refinements.
- The `.reference` webjail implementation observes zero-length responses
  and can hold a known-0 estimate; it is a reference only and is out of
  scope for this contract.
