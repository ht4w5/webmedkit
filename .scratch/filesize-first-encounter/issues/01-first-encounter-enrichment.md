# 01: First-encounter Filesize enrichment

**What to build:** A path's Filesize is known from its first observed
response. The enrichment run that observes an unobserved path creates
the record seeded exactly with the observation and enriches its own
event's size in the same run; later observations refine the estimate by
EWMA, and every run on an observed path carries the current estimate.
The warm-up gate is gone end to end: the enricher's construction loses
the minimum-samples parameter and the daemon's fixed file-size
parameters keep only the EWMA window. Replay protection accounts for a
path's very first request; an unknown size still means "never observed"
and replay still skips it blindly. Run outcomes follow the Enrichment
counter definition (first/later observation -> observed+enriched;
no observation on an observed path -> enriched only; any run on an
unobserved path -> neither).

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A successful observation on an unobserved path enriches the
      event's size in the same run (seeded exactly, no blending)
- [x] The minimum-samples parameter and its validation are gone from
      the enricher; the EWMA window stays fixed (alpha = 1/5) and
      unconfigurable
- [x] The daemon's fixed file-size parameters carry only the EWMA
      window
- [x] Later observations refine by EWMA and every run on an observed
      path carries the estimate; no flicker back to unknown while
      observations continue
- [x] Run outcomes match the Enrichment counter definition
- [x] Replay's blind skipping of unknown sizes is unchanged
- [x] Enrichment-package tests cover seeding, EWMA refinement, the
      outcome table, stale-value overwrite, and per-path state
      (warm-up-gate tests replaced by first-encounter cases)
- [x] E2E tests cover first-encounter enrichment feeding subsequent
      rules (enrich-then-replay tracks from request one) and
      enrichment-counter counts from the first observation

Implemented on integration branch `integration/filesize-first-encounter`
(merge commit `8aba3dd`; code-review refactors through `19010f4`, incl.
the `FilesizeCache.Get` rename and shared e2e send helper).
Context: spec `../spec.md` (Implementation Decisions; Further Notes).
