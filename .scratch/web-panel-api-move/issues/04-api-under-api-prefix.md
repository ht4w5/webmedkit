# 04: Move the machine API under `/api/`

**What to build:** The report API and the Statistic move to
`GET/DELETE /api/reports` and `/api/stats`; the Metric endpoint and the
liveness probe stay at the root as infrastructure endpoints. The old root
paths are gone (404) - no compatibility aliases. The report document
contract, the Statistic's free shape, and the DELETE semantics (rule and
target query parameters) are unchanged; only paths move. A report
consumer's operator updates one URL and their pipeline keeps working.
All shipped documentation - the consumer tool's docs and service-unit
examples, the report document contract page, the metrics page, the
README - and the pinned doc tests show the new paths.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `GET /api/reports` serves the unchanged report document; `DELETE /api/reports` removes a Report by (rule, Target) as before.
- [x] `GET /api/stats` serves the unchanged Statistic snapshot.
- [x] The old `/reports` and `/stats` root paths return 404.
- [x] `/metrics` and `/ping` are unmoved and unchanged.
- [x] Consumer docs, unit examples, and all pinned doc tests reference the new paths and pass.
- [x] The "panel at web root, machine API under /api" ADR records the trade-off against compatibility aliases.

## Answer

Implemented on `ticket/wam-04-api-under-api-prefix` (commit
`4097637`), merged into `integration/web-panel-api-move` (`2b74a84`).
`GET/DELETE /api/reports` and `GET /api/stats` serve the unchanged
report document and Statistic with unchanged DELETE semantics (rule +
target query parameters); the old `/reports` and `/stats` return 404
with no compatibility aliases; `/metrics` and `/ping` are unmoved. New
`apipaths_test.go` pins the relocation at the httptest seam and the
e2e unknown-path loop covers the old and new subpaths. All shipped
docs (README, docs/reports.md, metrics.md, ingest.md, rules.md,
storage.md, splint.md, configs/splint.service) and the pinned doc
tests show the new paths and pass. ADR
`docs/adr/0022-panel-at-web-root-machine-api-under-api.md` records the
trade-off against compatibility aliases. Reviewed in the fix pass
`62e3efa` (metrics.md Exposure now states the full unauthenticated
surface).
