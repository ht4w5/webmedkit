# Filesize is served, not learned - spec

Status: ready-for-agent

## Problem Statement

The Filesize of a path is *learned*: a filesize enrichment observes
eligible responses, smooths them through an EWMA per path, and stamps
the estimate onto later Events as a derived field. That design taxes
every user of the size signal:

- A crawl across rarely seen paths creates learned records nothing
  reads, and those records share the tracker store's budget - churn
  that can evict live tracker state.
- A path's first downloads replay-blind until its first observation
  lands, and the estimate needs a warm-up history the operator can
  neither see nor control.
- The estimate blends what the server happened to serve: gzip and
  identity representations mix, HEAD and 304 traffic skews what is
  observed, and the Sampling rule - a fixed eligibility predicate -
  is a second, invisible rule system living beside the Filter.
- Rules authors must know the documented composition (a tracker-less
  enrich rule ahead of the consuming rule) before a size predicate or
  a replay rule can see anything at all: plumbing before intent.
- The report sample exposes `content_length` and `content_range_total`
  - raw response-header residue of the learning pipeline - beside the
  single number downstream tooling actually wants.

## Solution

Filesize becomes **served, not learned**: the Filesize of an Event is
the file's size *as served with that response*, derived at parse time
from the fields the log already carries. There is no learned state, no
cache, no warm-up, no Sampling rule - and therefore no cache pollution,
no blind first downloads, and no blending across representations. Each
response reports its own size story.

The derivation is fixed and total:

- On a `206`, the Content-Range total is the only size source (the
  partial Content-Length would masquerade as a file size and shatter
  chunked downloads into full replay increments). An unusable or
  absent Content-Range reads as unknown, whatever the Content-Length
  says.
- On any other status, the Content-Length serves as Filesize when it
  is greater than zero; absent, empty or zero reads as unknown.
- Zero keeps meaning "unknown" and nothing else (ADR 0024 stands):
  `size unknown` selects exactly the responses without usable size
  information, and size comparisons match known sizes only.
- Validation keeps its deliberate asymmetry: a malformed
  Content-Length drops the line like any invalid field, while an
  unusual Content-Range degrades to no size and never drops the Event.

Because the sample is the whole Event, every report shows the served
size without further plumbing; because the log format is unchanged,
deploying this costs nothing on the emitter side - un-upgraded or
legacy lines simply read as unknown sizes.

The enrichment machinery itself - the enrich Action, its seams, the
Enrichment counters, their metric families and the Stats page table -
surfaces intact with an empty kind vocabulary, ready for future
enrichment kinds; only the filesize kind and all of its learned state
die. Existing rules files lose their filesize enrich rules: size
predicates and replay rules consume the parser-derived Filesize
directly.

## User Stories

1. As a rules author, I want Filesize to be derived from the response
   as served, so that a known size needs no learned state, no warm-up
   and no plumbing.
2. As a rules author, I want size predicates to work with no enrich
   actions anywhere in my file, so that "which traffic matters" is the
   only thing my rules express about size.
3. As a rules author, I want a `206` response to take its Filesize
   from the Content-Range total only, so that a resumed download
   counts its true fraction of the file and a chunk's partial length
   never masquerades as a file size.
4. As a rules author, I want a chunked download to saturate my replay
   ladder once - as one download - so that legitimate resumptions are
   not punished as a thousand downloads.
5. As a rules author, I want any other response to take its Filesize
   from its Content-Length when greater than zero, so that the size
   the server reports is the size my rules see.
6. As a rules author, I want responses without usable size information
   to read as unknown, so that `size unknown` selects exactly them.
7. As a rules author, I want zero to keep meaning unknown and nothing
   else, so that my existing `size >=`, `size <=` and `size unknown`
   Filters keep their established meaning.
8. As a rules author, I want a `HEAD 200` carrying a Content-Length to
   be a known size, so that "this file exists at this size" is
   representable even when nothing was transferred.
9. As a rules author, I want a malformed Content-Length to drop the
   line like any invalid field, so that junk can never fake a size.
10. As a rules author, I want an unusual Content-Range to degrade to
    unknown without dropping the line, so that odd headers cost me one
    unknown size, never a lost Event.
11. As a rules author, I want my replay rules to blind-skip unknown
    sizes exactly as before, so that un-sizeable traffic can never
    pollute my trackers.
12. As a rules author, I want the enrich Action to remain valid rules
    syntax with no kinds registered, so that my file is ready the
    moment a future kind lands.
13. As a rules author, I want any enrich Action to fail load with an
    error saying no enrichment types are available, so that a
    leftover `type: filesize` action is caught at load, loudly and
    helpfully.
14. As a rules author, I want the documented tracker-less enrich-rule
    composition to disappear from the docs, so that nothing teaches me
    plumbing that no longer pays rent.
15. As a rules author, I want the `bandage -t` plan to keep showing
    enrich actions inline like every other action, so that the plan
    contract is stable when kinds return.
16. As an operator, I want Filesize to need no cache entries at all, so
    that crawl bursts can no longer evict live tracker state.
17. As an operator, I want a path's first download to count at full
    weight immediately, so that replay protection is never blind while
    an estimate warms.
18. As an operator, I want each response to report its own
    representation's size, so that gzip and identity traffic can no
    longer blend into a skewed estimate.
19. As an operator, I want a `206` transfer to count as its true
    fraction of a download, so that a resumed file converges to one
    increment, not one per chunk.
20. As an operator, I want the report sample to carry `size_bytes` and
    stop carrying `content_length` and `content_range_total`, so that
    the wire contract reflects what the domain knows and nothing else.
21. As an operator, I want my stored reports to keep rendering after
    the two keys are gone, so that history survives the contract
    change.
22. As an operator, I want the legacy flux log lines to read as unknown
    sizes, so that mixed or un-upgraded emitters degrade to "replay
    stays blind", not to errors.
23. As an operator, I want the enrichment counters, their metric
    families and the Stats page table to survive the filesize kind's
    removal, so that observability is already in place for the next
    kind.
24. As an operator, I want the enrichment counter surfaces' explicit
    empty state, so that a kind-free daemon's silence is never
    mistaken for a broken panel.
25. As a scraper, I want the enrichment metric families unchanged in
    name, labels and shape, so that my dashboards survive the kind's
    removal untouched.
26. As a scraper, I want the metrics contract to stop describing the
    filesize kind's `observed` semantics, so that no HELP text
    documents a kind that cannot run.
27. As a deployer, I want zero rollout coupling: the bandage nginx log
    format keeps `clen` and `crange`, so upgrading the daemon alone
    changes everything and nothing on the emitter side.
28. As a maintainer, I want the Glossary to define Filesize as served
    size and retire the Sampling rule, so that the vocabulary matches
    the code the moment it changes.
29. As a maintainer, I want one ADR recording "Filesize is served, not
    learned", superseding the learning ADRs and amending the ones whose
    rationale bends, so that a future reader knows why learning died.
30. As a maintainer, I want the docs' stale promise of a load error
    "naming the migration" fixed while the enrichment section is
    rewritten, so that the documented error contract matches reality.
31. As a maintainer, I want the canonical report example and its
    reproduction pins updated in one change, so that docs, consumer
    mirror and stored-sample goldens cannot drift from the contract.
32. As a maintainer, I want every test of this area to assert external
    behavior - log lines in, reports and operator surfaces out - so
    that the suite survives future internal moves.

## Implementation Decisions

- The Filesize derivation is fixed and total, performed while parsing
  the access-log record into the Event:
  - `status == 206`: the Content-Range total is the only size source;
    a lenient parse yielding nothing greater than zero means unknown,
    and the Content-Length is ignored on a 206 even when present.
  - any other status: the Content-Length when greater than zero;
    otherwise unknown.
  - No method gating anywhere: `HEAD`, `404` and `POST` responses with
    a usable size are known sizes.
- The field-validation asymmetry is retained verbatim: a malformed
  Content-Length is an invalid field and drops the line; a malformed
  Content-Range degrades to no size and never drops the line. The
  asymmetry's justification changes (typed field vs free-form header)
  and is recorded as such.
- Filesize moves from the derived-field section of the Event to the
  original-field section: it is parse-time state. The single
  derived-field reset at evaluation entry is deleted; the convention -
  one reset for all derived fields - returns with the first future
  enrichment's derived field.
- The report sample's wire form is the whole Event, so removing the
  Content-Length and Content-Range total fields from the Event removes
  their keys from the wire contract: the canonical report example, the
  consumer-side sample mirror and the stored-sample goldens all lose
  `content_length` and `content_range_total`. The `size_bytes` key
  survives with its redefined meaning. Stored documents carrying the
  removed keys keep parsing - unknown keys are tolerated by
  construction.
- Zero stays the unknown marker; "known empty" stays unrepresentable
  (ADR 0024's contract stands on the new rationale).
- Replay semantics are preserved structurally and reinterpreted
  semantically: unknown sizes blind-skip the tracker exactly as before;
  the increment is the transferred fraction of this response's served
  size, capped at 1. A 206 keeps counting its true fraction of the
  download; a full response counts 1.
- The filesize enrichment is deleted entirely: the enricher, its
  learned state (EWMA record), its cache port and store adapter, the
  build-level EWMA window constant, its cache namespace and the
  TTL reservation that protected it. The Sampling rule retires with it.
- The enrichment plumbing survives with an empty kind vocabulary: the
  enrich Action and its rules parsing, the enrichment seam and counter
  seam, the global registry and the builder vocabulary (kept, empty),
  the per-(rule, kind) Enrichment counters, their three metric
  families, the `/api/stats` enrichment block and the Stats page table.
  ADR 0028 stands.
- With an empty vocabulary, every enrich Action fails load with an
  error naming the type and stating that no enrichment types are
  available - the action stays parseable and documented as reserved
  plumbing.
- Shipped and example rules files drop their filesize enrich rules;
  their size predicates and replay rules now read the parser-derived
  Filesize directly. The stale "load error names the migration"
  commentary is corrected: removed syntax rides the strict decoder's
  generic refusal, as ADR 0028's review note already established.
- Record-keeping in the same effort: the Glossary redefines Filesize
  as served size and removes the Sampling rule; the ingest, rules,
  metrics and reports documents move to the served-size story; one new
  ADR - "Filesize is served, not learned" - covers both the semantic
  flip and the two wire keys, superseding the two learning ADRs and
  amending the two ADRs whose rationale bends.

## Testing Decisions

- A good test asserts external behavior only: raw log lines in,
  operator-visible outcomes out - report samples with `size_bytes`,
  replay accumulation through reports and stats, Filter matches, drop
  counts, load errors, plan text. No test reaches into parse internals
  or enrichment plumbing internals.
- Primary seam: the assembled-daemon end-to-end seam - rules YAML as
  the input surface, a started test daemon fed synthetic access-log
  lines, outcomes read from the operator surfaces. Prior art: the
  existing size-estimate, sampling-admit-reject, resumed-download,
  malformed-clen and unusual-crange end-to-end suites and the
  replay-ladder tests, rewritten as served-size behavior. This seam
  carries the derivation table's behavior, the 206-fraction accounting,
  the unknown-size blind skip and the empty-vocabulary load errors.
- Existing unit seam kept (no new seams): the parser's field-contract
  table test - derivation table, strict-Content-Length /
  lenient-Content-Range asymmetry, drop-line accounting. Prior art:
  the parser's existing field tests, re-pointed at the served size.
- Contract pins updated in place, not extended: the report document
  reproduction test, the consumer-side sample mirror pin, the
  stored-sample goldens and the metrics document reproduction test all
  exist and merely lose two keys and one kind's semantics.
- The suite runs race-enabled; replay fraction behavior over chunked
  downloads is asserted end-to-end at the assembled-daemon seam.

## Out of Scope

- Any new enrichment kind; making the enrich Action useful before a
  kind exists. The plumbing is reserved, nothing more.
- Representing "known empty" (size 0 known to be an empty file); zero
  stays the unknown marker.
- Per-representation size tracking (separating gzip from identity
  sizes); each response reports its own size and that is accepted.
- Any change to the emitter-side log format: `clen` and `crange` stay
  in the bandage nginx log format unchanged.
- Migration of stored report rows; old rows carry extra keys and
  historic size meanings, tolerated as-is within their TTL.
- Replay mathematics beyond the semantic reinterpretation (increment
  formula, cap, blind skip, TTL derivation all stand).
- Removing or renaming the enrichment counter surfaces, metric
  families or panel table.

## Further Notes

- Rollout is deliberately un-coupled: deploying the new daemon against
  an unchanged log format flips the semantics in one step; legacy flux
  lines simply read as unknown sizes, so replay stays blind on
  un-upgraded traffic exactly as before.
- The gzip-blending and HEAD-observation consequences of the learning
  ADRs die with the learning model; "sizes as served" is now literal -
  each Event's Filesize is the size of the representation that
  response carried.
- The wire contract's key removal is the one irreversible-feeling
  part; it is safe because nothing acts on the two keys downstream and
  unknown keys are tolerated on read.
