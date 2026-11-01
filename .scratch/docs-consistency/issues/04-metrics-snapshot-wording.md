# 04: docs/metrics.md snapshot wording

**What to build:** `docs/metrics.md:19-20` only. The line "they agree by
construction, because one snapshot feeds both" is wrong: `/stats` and
`/metrics` each snapshot per request. Reword to say each response is
internally consistent, but two separate requests can disagree.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent
