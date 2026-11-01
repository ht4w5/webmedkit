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

- [ ] A Report carries the Event value as its Sample; no
  sample-specific field list exists anywhere in the daemon
- [ ] The sample's JSON form is derived from the Event type: the
  emitted field set comes from the type definition (never a
  hand-written list), timestamps render RFC 3339 UTC at second
  precision with sub-second truncated, and the client address
  round-trips in text form
- [ ] The served report document is byte-for-byte unchanged - the
  canonical example in the contract document needs no regeneration
- [ ] A field added to the Event appears in the marshalled sample
  without any report-side change
- [ ] The report body a Trigger persists and the document the daemon
  serves render the sample identically, so a persisted report
  survives a restart unchanged
