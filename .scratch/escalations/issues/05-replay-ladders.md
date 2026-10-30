# 05: Replay ladders

**What to build:** A ladder on a replay tracker: thresholds in download
equivalents - already normalized by Filesize, so `2` means two full
downloads of that path whether it is a 1MB or a 10GB file - with the same
firing, terminality and override semantics as rate and bandwidth ladders.
Warm-up blindness is preserved: events whose Filesize is unknown skip the
tracker entirely (no increment, no decay). The cache-entry TTL re-derives
per event from the ladder's top rung, so a replay ladder's state outlives
the climb exactly like the other kinds. Volume methods (`constant`,
`clamped-linear`, `inverse-square`, `step`) stay exclusive to the
single-volume form, and load says so.

**Blocked by:** 03

**Status:** resolved

- [x] A replay tracker accepts `escalations` with plain-number thresholds
      in download equivalents
- [x] Events with unknown Filesize skip the tracker entirely (no
      increment, no decay, no cache touch)
- [x] The cache-entry TTL re-derives per event from the ladder's top rung
- [x] Load refuses a volume-method block on a ladder rule and a ladder on
      a single-volume rule, each error naming the rule
- [x] Fired rungs, per-fired-rung terminality and report override behave
      exactly as for rate and bandwidth ladders

Implemented on integration branch `feat/escalations` (commit `338f766`;
cross-ticket code-review fix pass through `129c5ea`). Replay ladders
take plain-number thresholds in download equivalents, keep warm-up
blindness (unknown-Filesize events touch no cache), re-derive the
cache-entry TTL per event from the ladder's top rung, and load refuses a
volume-method block beside a ladder and a ladder beside
`tracker.volume`, each error naming the rule. Fired rungs, per-fired-
rung terminality and report behaviour ride the shared rung machinery,
identical to rate and bandwidth ladders.

Context: spec `../spec.md` (decision 3, TTL paragraph); builds on
ticket 03's rung dispatch.
