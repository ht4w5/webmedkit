# 03: Sample keys mirror the Event's names

**What to build:** The sample keys state their units and match the
Event's own field names: `sent` becomes `sent_bytes` and `size`
becomes `size_bytes` across the contract document and its example,
splint's mirror, and every test and bench fixture. Reports persisted
before the reshape keep serving: the renamed fields read back as
absent (zero), so a daemon upgrade never drops a live report, and
nothing dual-reads the old keys.

**Blocked by:** 01 - The Sample is the Event, wire-identical.
(Deliberately not 02: the two reshapes can land in either order.
Work them sequentially anyway - both regenerate the canonical
example and would conflict textually if run in parallel.)

**Status:** resolved

- [x] Every mention of the sample keys `sent` and `size` in the
  contract and consumer surface is now `sent_bytes` / `size_bytes`,
  mirroring the Event's field names; the sample key order follows
  the Event's field order
- [x] A report body written in the pre-reshape shape reads back with
  `sent_bytes` and `size_bytes` zero (zero means absent); no code
  reads the old keys for compatibility
- [x] The byte-for-byte contract test is green against the
  regenerated canonical example
- [x] The `limit-bandwidth` stance is documented against the bytes
  the sample counts under `sent_bytes`
- [x] No report document, persisted report body, or fixture carries
  the old key names

## Answer

Implemented on `ticket/whole-event-03-sample-key-renames` (commits
`178ed93`, `2c8bc1d`), merged into `integration/whole-event-sample`
(`4a5b4f9`). `sent`/`size` are `sent_bytes`/`size_bytes` across the
contract document and its regenerated example, splint's mirror, tests
and fixtures; `TestPreReshapeBodyKeepsServing` pins the legacy-row
behaviour (a pre-reshape body reads back with the renamed and added
fields zero - zero means absent - and nothing dual-reads the old
keys); the `limit-bandwidth` prose in `docs/reports.md` and
`docs/rules.md` now references `sent_bytes`. Review fix pass through
`bd2d5fe`.
