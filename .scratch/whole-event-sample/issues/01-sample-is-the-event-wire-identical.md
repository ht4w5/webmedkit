# 01: The Sample is the Event, wire-identical

**What to build:** A Report carries the Event value itself as its
Sample - the shadow sample type and its hand-copied field list are
gone - and the sample object of the report document is produced from
the Event type rather than a written-out field list. From the outside
nothing changes: the served document is byte-for-byte identical,
still the same six sample keys (`time`, `path`, `agent`, `status`,
`sent`, `size`). This is the prefactor that makes the easy change
easy: the sample's field set now follows the Event by construction.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [x] A Report carries the Event value as its Sample; no
  sample-specific field list exists anywhere in the daemon
- [x] The sample's JSON form is derived from the Event type: the
  emitted field set comes from the type definition (never a
  hand-written list), timestamps render RFC 3339 UTC at second
  precision with sub-second truncated, and the client address
  round-trips in text form
- [x] The served report document is byte-for-byte unchanged - the
  canonical example in the contract document needs no regeneration
- [x] A field added to the Event appears in the marshalled sample
  without any report-side change
- [x] The report body a Trigger persists and the document the daemon
  serves render the sample identically, so a persisted report
  survives a restart unchanged

## Comments

Ticket 01 landed on `ticket/whole-event-01-sample-is-the-event`
as the wire-identical prefactor: `domain.Sample` and its hand-written
marshal/unmarshal are deleted, `Report.Sample` is `domain.Event`, and
the sample's JSON form is derived from the Event via the
`type sampleWire Event` shim plus a timestamp renderer
(formatTime/parseTime). The four non-contract fields (RemoteAddr,
Method, ContentLength, ContentRangeTotal) are parked in the Event's
wire form with `json:"-"` tags; the served document and the persisted
body still carry exactly the six keys `time, path, agent, status,
sent, size`, byte-for-byte (docs/reports.md untouched).

On the checklist's address clause: the address-rendering machinery
(netip.Addr in text form, zero <-> empty string - netip's text
marshalling) is in the derived wire form now, so ticket 02 is purely
additive (flip the parked tags); it becomes observable at the document
seams when ticket 02 un-parks the fields, as the decomposition
resolves it. Similarly, "a field added to the Event appears without a
report-side change" is by construction (the sample's emitted field set
lives only in the Event's tags); ticket 02's tag-only diff to un-park
four fields is the demonstration.

Note for tickets 02/03: the Event<->wire coupling ADR 0021 records is
deliberate and visible beyond the report - the log action's `event`
attribute now renders in the Event's wire form too (six keys,
second-precision time), so `TestE2ELogActionLogsEventAndTrackerReading`
was updated to the wire form. When 02 un-parks the fields, that log
attribute regains method/remote_addr/content_* without further code.
