# 06: Surface per-prefix Meter hits in outcome logs

**What to build:** With per-prefix meters, each effective Target prefix's
chain carries its own counter, but outcome logs keep reporting only
"since the last change" totals per ban rule. Surface the per-prefix
breakdown - e.g. extend the install log line (or the outcome the driver
returns) with per-prefix packet/byte counts - so an operator can see *which*
limited prefix is consuming its budget. Per-ban-rule totals must remain as
they are today.

**Blocked by:** 04: Per-prefix Meters for limit-bandwidth - both stances'
chain counters and the chain-aware hit read-back land there.

**Status:** ready-for-human

- [ ] Decide the output shape (log fields vs. outcome structure) - this
      extends a contract the repo treats as curated, so the shape wants a
      human call.
- [ ] Extend the driver's hit read-back to carry per-prefix counts (already
      parsed from chain names) up to the outcome.
- [ ] Document the new fields wherever the install-log format is described.

## Comments

Filed from the per-prefix meters design interview (2026-06); accepted as a
follow-up when the counters moved into the meter chains. Renumbered from 02
when implementation tickets landed ahead of it.
