# The sample is the whole event

A report's sample is the **complete Event** that justified its current
stance - every field, including the exact client address behind an
aggregated Target and the response-size observations that feed the
Filesize estimate - never a curated projection of "interesting" fields.
In code the sample *is* the domain `Event` value (the term Sample stays
the glossary role), and the report document's `sample` object mirrors
the Event's field names (`sent_bytes`, `size_bytes`, ...). The reshaped
document stayed at `version` 0: it had no consumers yet.

Considered: keeping a curated `Sample` struct of selected fields
(rejected: every Event extension becomes a hand-edit and the projection
silently rots - which is exactly how the six-field shadow of a
ten-field Event happened); a distinct `Sample` type mirroring all
fields with a reflection completeness test (rejected: that is
detection, not inclusion - the plan asked for future Event fields to
land in reports automatically); leaving `RemoteAddr` out because
Target aggregates the source (rejected: triage needs the individual
host, and aggregation is a tracker-key concept, not a privacy
guarantee - the address is already in the access log).

Consequences: `Event` now carries the report contract's wire shape
(JSON tags, RFC 3339 UTC second-precision timestamps), a coupling that
is deliberate; that contract wire form is the Event's sole JSON form,
so anything marshalling an Event - the log action's `event` attribute
included - renders it; new Event fields appear in reports by
construction, and the byte-for-byte canonical-example test in
`docs/reports.md` forces the contract document to be updated with
them; served documents disclose the unaggregated client address; rows
persisted before the change read back with zeroed renamed and added
fields (zero means absent - no dual-read). Recorded because the sample's completeness is
a stance a future reader will be tempted to "optimize" back into a
projection.

Amended by ADR 0032: the whole-event stance stands; the Event simply
carries two fewer fields. `content_length` and `content_range_total` -
the response-size observations this decision named - leave the Event
with the learning pipeline, so the sample and the log action's `event`
attribute lose the two keys while `size_bytes` stays, now the served
Filesize. Stored documents carrying the removed keys read back as
before: unknown keys are tolerated by construction.
