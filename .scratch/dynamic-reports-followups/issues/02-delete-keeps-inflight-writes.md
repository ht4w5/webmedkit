# 02: Delete keeps in-flight writes in the live set

**What to build:** `report.Service.Delete` drops matching live-set
entries while a write for the pair is in flight, orphaning the entry's
in-flight channel - a Trigger waiter only wakes when the writer closes
it. `Delete` leaves in-flight pairs standing (their writes land after
the delete and suppress normally - the delete never saw their rows),
matching `DeleteExpired` and `DeleteDynamic`'s skip discipline; settled
pairs still leave, re-arming writing. The channel close stays the
writer's alone: exactly once, on every settle and failure path, so no
waiter can ever hang.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A Delete during an in-flight write leaves the landed report
      standing: suppression intact, no duplicate write from a waiter
- [ ] Every waiter is released when the write settles or fails; the
      channel close is unconditional on every writer path
- [ ] Settled pairs still leave the live set on Delete (re-arm
      writing), unchanged
