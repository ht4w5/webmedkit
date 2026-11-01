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

- [ ] The served sample object carries `method`, `remote_addr`,
  `content_length` and `content_range_total`, taken from the
  Trigger's Event; `content_*` render `0` when absent or unusable
- [ ] The contract document's sample table documents the four keys
  and the zero-means-absent convention, and its canonical example
  shows a populated full sample; the byte-for-byte contract test is
  green against it
- [ ] The report document stays at `version` 0
- [ ] splint's report type mirrors the added fields without acting on
  them, and a splint cycle over the canonical example stays green
- [ ] A higher rung's Trigger overrides a live report with the whole
  sample - including the new fields - moving to the overriding rung's
  Event, while `created_at` stays frozen
