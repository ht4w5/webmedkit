# 04: Report rungs and in-place override

**What to build:** A rung's report action carries its own `reason`,
`penalty`, `rate` and `ttl` - the ladder's responses escalate from a
throttle recommendation to a ban under whatever codes the author chooses
(distinct or shared across rungs). A Trigger from a strictly higher rung
**overrides the live Report in place**: reason, penalty, rate, Sample,
tracker reading, `updated_at` and `expires_at` all move to the overriding
rung's values, so a report consumer always sees the abuse as it stands -
a `reject` ban backed by the event that justified the ban, living for the
ban rung's `ttl`. `created_at` alone stays frozen at first flagged. A
Trigger from a rung at or below the threshold recorded on the live Report
is suppressed - a ban can never soften back into a throttle. When a
Report dies and the accumulation still stands above several rungs, the
next matching event writes at the lowest crossed rung and overrides up the
crossed rungs within that same event.

**Blocked by:** 02, 03

**Status:** resolved

- [x] A rung's report action validates exactly like a rule's report
      action today (reason slug, ttl, penalty, rate rules)
- [x] A Trigger from a strictly higher rung than the recorded threshold
      overrides the live Report in place: reason, penalty, rate, Sample,
      tracker reading, `updated_at` and `expires_at` take the rung's
      values
- [x] `created_at` never moves across writes
- [x] A Trigger from a rung at or below the recorded threshold is
      suppressed - no downgrade and no rewrite, ever
- [x] The guard is the volume recorded in the Report's tracker reading
      (the writing rung's threshold) and holds across a daemon restart
      (the live set is rebuilt from the report table)
- [x] With the value above several rungs and no live Report, one matching
      event produces one live Report at the top crossed rung's stance
      (writes at lower crossed rungs are overridden within the event)
- [x] The report contract's escalation prose lands in the same change and
      its byte-pinned example passes untouched

Implemented on integration branch `feat/escalations` (commit `4577e17`;
cross-ticket code-review fix pass through `129c5ea`). A Trigger from a
rung strictly above the threshold recorded in the live Report's tracker
reading overrides the row in place - reason, penalty, rate, Sample,
reading, `updated_at` and `expires_at` move to the rung's values,
`created_at` stays frozen - while a rung at or below is suppressed, so a
ban never softens. The guard rides the recorded `tracker.volume` and
survives the live-set rebuild at startup; a dead Report's next event
writes at the lowest crossed rung and overrides up within that event.
The report contract's escalation prose landed in the same change with
its byte-pinned example untouched.

Context: spec `../spec.md` (decisions 1, 5, 7, 8); ADR 0013.
