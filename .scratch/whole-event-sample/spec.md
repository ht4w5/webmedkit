# Full-event samples in reports

Status: ready-for-agent

**Origin:** plan request - "for bandage reports, attach the full event
as the sample, not just a few fields" - settled through a
grill-with-docs session, then drafted to spec.

## Problem Statement

When an operator triages a bandage Report, the Sample is supposed to be
the Event that justified the report's stance - but the report only
carries six of the Event's ten fields. The request method and the exact
client address are missing, as are the response-size observations
(`Content-Length`/`Content-Range`) that fed the Filesize estimate the
report's `size` asserts. Triaging "why does this report say the file is
6000 bytes, and which host behind this `/24` actually did it?" means
digging through raw access logs, because the report deliberately
withholds the answer. Worse, the Sample is a hand-mirrored shadow type:
every future field added to the Event silently stays out of reports
until someone remembers the projection.

## Solution

A report's Sample **is** the whole Event: every field, carried as-is,
with the report document's `sample` object mirroring the Event's field
names. The sample JSON form is derived from the Event type itself, so
future Event extensions appear in reports automatically and the
published contract example has to document them (the byte-for-byte
contract test enforces this). The wire reshapes while it still has no
consumers: `sent` and `size` become `sent_bytes` and `size_bytes`, and
`method`, `remote_addr`, `content_length`, `content_range_total`
arrive. Document `version` stays `0`; reports persisted before the
change read back with the new fields absent (zero means absent).

## User Stories

1. As an operator triaging a Report, I want the Sample to carry the
   request method, so that I can tell a download storm from a header
   flood without leaving the report.
2. As an operator triaging a Report, I want the Sample to carry the
   exact client address, so that I can identify the individual host
   behind an aggregated Target prefix.
3. As an operator investigating a Filesize estimate, I want the Sample
   to carry the response's Content-Length and Content-Range total,
   so that I can see which observations fed the estimate the report
   asserts.
4. As an operator reading a Report, I want the Sample to be the
   complete Event - never a curated subset - so that the report
   answers "what happened?" on its own.
5. As a report consumer, I want the `sample` object's keys to mirror
   the Event's field names, so that one vocabulary spans the daemon's
   internals and the wire.
6. As a report consumer, I want `sent` and `size` renamed to
   `sent_bytes` and `size_bytes`, so that units are stated in the
   contract instead of remembered.
7. As a report consumer, I want absent observations rendered as
   zeroes (`0` / empty string), so that "absent" has one convention
   across the document.
8. As a report consumer, I want the document to stay at `version` 0
   through the reshape, so that version semantics stay reserved for
   changes that break consumers.
9. As a splint maintainer, I want splint's report decoding to mirror
   the reshaped contract fields even though it does not act on them,
   so that the published document remains the single integration
   surface.
10. As a splint maintainer, I want the canonical example to remain a
    runnable splint cycle, so that consumer tolerance of the reshaped
    sample is proven, not assumed.
11. As an operator with an existing deployment, I want reports written
    before the change to keep serving, so that a daemon upgrade never
    drops live reports - missing sample fields simply read as absent.
12. As an operator, I want an escalated Report's Sample to move to the
    overriding rung's Trigger whole, so that the sample always backs
    the Penalty the report carries.
13. As an operator, I want Trigger semantics untouched - one live
    report per (rule, Target), superseded sample on escalation - so
    that no lifecycle behaviour changes under this reshape.
14. As a rules author, I want the sample's `sent_bytes` to keep exactly
    the meaning the `limit-bandwidth` Penalty references, so that the
    advisory rate and the evidence stay comparable.
15. As a developer extending the Event, I want a new Event field to
    land in every report automatically, so that completeness never
    depends on remembering a projection.
16. As a developer extending the Event, I want the byte-for-byte
    contract test to fail until the published example documents the
    new field, so that the contract cannot silently drift.
17. As a developer, I want the shadow `Sample` type gone, so that
    there is no second field list to rot.
18. As a developer, I want the report document and the persisted
    report body to use one sample wire form, so that serving and
    storage cannot disagree about what a sample is.
19. As a reviewer, I want the reshape recorded as an ADR and the
    Sample sharpened in the glossary, so that nobody "optimizes" the
    sample back into a projection later.
20. As a privacy-conscious operator, I want the raw-client disclosure
    to be a recorded, deliberate decision, so that it can be revisited
    knowingly rather than discovered.

## Implementation Decisions

- **Sample ≡ Event.** The `Sample` type is deleted; a Report carries
  the Event value itself. "Sample" remains the glossary role and the
  document field name. This is the whole point: there is no field
  list left to forget (ADR 0021).
- **Wire form derived from the struct.** The Event's JSON form is
  produced from the type definition (tags plus a timestamp renderer),
  never from a hand-written field list. A defined-type shim over the
  Event keeps the emitted field set in lockstep with the type by
  construction; a reflection walk is the acceptable fallback if the
  shim misbehaves. Timestamps render RFC 3339 UTC at second
  precision, sub-second truncated; the client address round-trips in
  its text form.
- **Wire key names mirror the Event's.** Renames: `sent` →
  `sent_bytes`, `size` → `size_bytes`. New keys: `method`,
  `remote_addr`, `content_length`, `content_range_total`
  (`content_*` are the server-reported Content-Length and the total
  from Content-Range; `0` means absent or unusable, matching the
  Event's own convention). `time`, `path`, `agent`, `status` are
  unchanged. Sample key order follows the Event's field order.
- **`version` stays `0`.** A pre-consumer reshape is not a version
  event; the contract keeps its rule that additive changes never bump
  it, and ADR 0021 records that the shape moved once before
  consumers existed.
- **No migration, no dual-read.** Rows persisted before the change
  read back with the renamed and added sample fields zeroed: zero
  means absent. Old keys are not read for compatibility.
- **Unchanged semantics.** Trigger, escalation override, expiry and
  Penalty behaviour are untouched: an override moves the whole sample
  to the overriding rung's Trigger, as it moved the six fields
  before. The `limit-bandwidth` stance keeps referring to the bytes
  the sample counts, now under `sent_bytes`.
- **One wire form for serve and store.** The report document and the
  persisted report body marshal the sample identically.
- **splint mirrors the contract.** The consumer's report type gains
  the renamed and added fields; it continues to act on none of them -
  ban-rule derivation stays Target and Penalty only.
- **Domain docs.** GLOSSARY's Sample entry already sharpened to "the
  complete Event ... never a projection of selected fields";
  ADR 0021 records the decision and the rejected alternatives.

## Testing Decisions

A good test here asserts the **external shape of the report document
and its durability** - the observable contract - and never the
marshalling internals (no tests of shims, tags, or reflection). The
fewer seams the better; this feature is proven at three existing ones,
no new seams introduced:

- **The report document contract (primary).** The existing
  byte-for-byte test that drives the canonical rules scenario and
  compares `GET /reports` against the first fenced `json` example in
  the report contract document. Everything lands here: the renamed
  keys, the four added fields, and - because the emitted field set is
  derived from the Event - any future Event extension. It is also the
  docs-sync guard: the example must be regenerated with the reshaped
  sample (method, client address, and content observations populated
  in the scenario).
- **The report repository round-trip (persistence).** The existing
  sqlite repository tests extend to cover the reshaped sample through
  the body blob, plus one test writing a pre-change-shape body and
  reading it back with the missing fields zero (zero means absent).
- **splint's cycle ingest (consumer tolerance).** The existing cycle
  test feeds the canonical example through a splint cycle; updating
  the fixture proves the consumer tolerates the reshaped sample.
  Prior art: the same test today proves tolerance of the current
  example.

Prior art also includes the domain JSON round-trip tests, which keep
covering marshal/unmarshal mechanics (timestamp truncation, absent
values) as unit-level behaviour.

## Out of Scope

- splint *acting* on any sample field: ban-rule derivation remains
  Target + Penalty; sample fields stay mirrored-but-inert.
- Any change to Target aggregation, Tracker semantics, Trigger /
  escalation lifecycles, expiry, or Penalty values.
- Redacting or aggregating `remote_addr` in the served document -
  the disclosure is accepted and recorded, not mitigated (a future
  policy change would be its own effort).
- Version-bump policy for future breaking contract changes.
- Event fields beyond the report's Sample (no general event log or
  event export exists or is added).
- Metrics, statistics, and the rules DSL: untouched.

## Further Notes

- ADR 0021 ("the sample is the whole event") records the trade-off:
  completeness and by-construction freshness against document size,
  raw-client disclosure, and the deliberate coupling of the Event
  type to its wire form.
- The byte-for-byte contract test doing double duty as the
  completeness guard is the reason no reflection-based "does Sample
  mirror Event?" test is needed - and the reason the wire form must
  be derived from the struct rather than hand-listed.
- Implementation is tracked as tracer-bullet tickets 01–03 in this
  feature directory, each a vertical slice through domain, contract,
  consumer and tests: the wire-identical Sample≡Event prefactor, the
  additive whole-Event fields, then the key renames with the
  legacy-row behaviour.
