# 02: Reports carry the whole Event: the four missing fields

**What to build:** Every report's Sample now shows the request
method, the exact client address behind the aggregated Target, the
response's Content-Length, and the Content-Range file total. The
operator answers "which host, what request, why this Filesize
estimate" from the report alone - no digging through access logs. The
change is purely additive to the report document (the contract's
blessed path: `version` stays 0). The canonical example is
regenerated with a full sample, and splint mirrors the added fields
and still ingests the example green.

**Blocked by:** 01 - The Sample is the Event, wire-identical

**Status:** ready-for-agent

- [x] The served sample object carries `method`, `remote_addr`,
  `content_length` and `content_range_total`, taken from the
  Trigger's Event; `content_*` render `0` when absent or unusable
- [x] The contract document's sample table documents the four keys
  and the zero-means-absent convention, and its canonical example
  shows a populated full sample; the byte-for-byte contract test is
  green against it
- [x] The report document stays at `version` 0
- [x] splint's report type mirrors the added fields without acting on
  them, and a splint cycle over the canonical example stays green
- [x] A higher rung's Trigger overrides a live report with the whole
  sample - including the new fields - moving to the overriding rung's
  Event, while `created_at` stays frozen

## Comments

Ticket 02 landed on `ticket/whole-event-02-whole-event-fields` as the
additive whole-Event fields: the four parked Event fields are
un-parked in the wire form (`remote_addr`, `method`,
`content_length`, `content_range_total`, in the Event's field order;
the `sent`/`size` keys are unchanged - the renames are ticket 03),
and the canonical example in `docs/reports.md` is regenerated from a
driven run (byte-for-byte procedure: run
`go test -run TestE2EReportsDocumentMatchesDocsExample ./bandage/internal/app/ -v`
and copy the logged `GET /reports` body into the first fenced block).
The sample table documents the four keys and the zero-means-absent
convention; `version` stays 0.

The example's scenario now sends `clen`/`crange` on its two log lines
(two 6000-byte ranges of a 3000000-byte file), so the sample carries
the content observations. The Filesize estimate renders `size: 0` -
faithful to behaviour: the enricher's warm-up gate is five samples and
the scenario overflows on its second event (warm-up semantics are out
of scope to change), so the example shows what the daemon serves.

splint's report type mirrors the four fields mirrored-but-inert (ban
derivation stays Target + Penalty only), pinned by a `ParseDocument`
decode of the canonical example; the splint cycle suite runs green
over the updated fixture (the example is the shared fixture). The
escalation override assertions pin that a higher rung's Trigger moves
the whole sample - new fields included - to the overriding rung's
Event while `created_at` stays frozen, and the log-action contract
test pins the logged Event's whole wire form at second-precision time
(the ADR 0021 Event↔wire coupling the ticket 01 notes flagged).
