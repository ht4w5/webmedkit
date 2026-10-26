# 02: Triggers suppress, never reset the tracker

**What to build:** A live Report is the only thing that bounds report
writes (ADR 0013, amending ADR 0008). Repeated overflow while the Report
is alive writes nothing **and empties nothing** — the tracker keeps
accumulating, so a Tracker used as a threshold gate never flickers shut
on a write. When the Report dies (expiry or deletion), the next matching
event writes a fresh Report at the accumulation's current stance: its
Sample and tracker reading describe that event, its timestamps stamp the
write. The rules guide's Trigger semantics and the report contract's
lifecycle prose are updated in the same change, so the contract never
describes unshipped behaviour.

**Blocked by:** 01

**Status:** resolved

- [x] A Trigger whose write lands leaves the tracker record intact: the
      value keeps accumulating across writes
- [x] A Trigger whose write is suppressed (live Report) leaves the
      tracker record intact and the Report untouched
- [x] While a Report is alive, exactly one report row exists per
      (rule, Target) and its `updated_at` does not move
- [x] After expiry or deletion, the next matching event writes a fresh
      Report whose Sample and tracker reading come from the current
      accumulation
- [x] A failed report write still leaves the tracker record intact and
      retries on the next matching event
- [x] The trigger-cadence test is rewritten from "a Target Triggers once
      per volume of traffic" to "one write per report lifetime"
- [x] The rules guide's Trigger semantics and the report contract's
      lifecycle wording match the new behaviour in the same change

Implemented on integration branch `feat/escalations` (commit `90244c0`;
cross-ticket code-review fix pass through `129c5ea`). A Trigger now
suppresses or writes and never empties the tracker record
(`TrackerCache.Clear` deleted); a dead Report re-arms a fresh write at
the accumulation's current stance and a failed write retries with the
record intact. The trigger-cadence suite is rewritten to "one write per
report lifetime"; the rules guide's Trigger semantics and the report
contract's Lifecycle prose landed in the same change. The code-review
pass closed the volume-method corner: rules without a ladder never
override their live Report (`updated_at` frozen for the lifetime).

Context: spec `../spec.md` (decision 6, amending ADR 0008); ADR 0013.
