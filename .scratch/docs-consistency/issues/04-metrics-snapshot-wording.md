# 04: docs/metrics.md snapshot wording

**What to build:** `docs/metrics.md:19-20` only. The line "they agree by
construction, because one snapshot feeds both" is wrong: `/stats` and
`/metrics` each snapshot per request. Reword to say each response is
internally consistent, but two separate requests can disagree.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Comments

- **Context (implementer):** Reworded `docs/metrics.md`'s "agree by
  construction, because one snapshot feeds both" to: each endpoint takes
  its own point-in-time snapshot per request, so every response is
  internally consistent, but two separate requests (one per surface) are
  two snapshots and can disagree. Docs-only; `make test` passes (the
  pinned example and family table are untouched).
