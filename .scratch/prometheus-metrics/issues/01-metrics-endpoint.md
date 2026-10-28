# 01: Scrapeable metrics endpoint (stats mirror)

**What to build:** An operator points a Prometheus scraper at `GET
/metrics` on the daemon's existing HTTP listener and reads the daemon's
pipeline and tracker-cache statistics as `bandage_`-prefixed metric
families in Prometheus text exposition 0.0.4 — the same
cumulative-since-start values the human-facing Statistic at `/stats`
serves, plus start time and build identity — with `# HELP` and `# TYPE`
on every family and correctly escaped label values. The exposition is
rendered by a pure snapshot-to-text mapping (no client library, ADR
0014), with each family's name, type, help text and source single-sourced
in one table, and the full response is pinned byte-exact by a golden test
over fixed injected snapshots.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `GET /metrics` on the existing listener serves the families:
  `bandage_start_time_seconds`; `bandage_build_info` (gauge, value 1,
  labels carrying the full build identity: name, version, commit, build
  date, dirty, Go version, platform);
  `bandage_pipeline_events_received_total`,
  `bandage_pipeline_events_parsed_total`,
  `bandage_pipeline_events_dropped_total`;
  `bandage_pipeline_workers`;
  `bandage_tracker_cache_gets_total`, `_sets_total`, `_misses_total`,
  `_evictions_total`;
  `bandage_tracker_cache_entries`, `bandage_tracker_cache_entry_cap`
- [x] Every family is preceded by `# HELP` and `# TYPE`; families appear
  in a fixed deterministic order; label values are escaped per the
  exposition spec (quotes, backslashes, newlines)
- [x] The response content type declares the exposition format version
  (`text/plain; version=0.0.4; charset=utf-8`); non-GET methods are
  rejected
- [x] Metric values agree with the `/stats` snapshot by construction —
  one snapshot feeds both surfaces
- [x] Counters are cumulative since start; the renderer depends only on
  the standard library (no new dependencies)
- [x] Tests at the wire seam: a golden byte-exact full response over
  fixed injected snapshots (family order included), a nasty-label
  escaping case, and an endpoint round-trip asserting the content type

## Answer

Implemented on integration branch `feat/prometheus-metrics` (merge
commit `4eed1dd`; cross-ticket code-review fixes through `7fa5a22`).
`GET /metrics` on the existing listener serves the twelve `bandage_`
families in exposition 0.0.4 from `httpapi.metricFamilies` — one table
single-sourcing name, kind, help and snapshot source — rendered by the
pure `renderMetrics` from the same `httpapi.Stats` snapshot `/stats`
serves (existing `StatsSource` port reused), so the two surfaces agree
by construction. Golden byte-exact response pinned at the wire seam
(`metricsGoldenResponse` over `metricsFixture`); label escaping, HELP/
TYPE on every family, content type and method rejection pinned beside
it. Non-GET *including HEAD* answers 405 with `Allow: GET` (the
code-review pass caught HEAD slipping through ServeMux's GET pattern).

Context: spec `../spec.md`, `docs/adr/0014-metrics-are-a-hand-rendered-machine-contract.md`.

Notes:
- `StartedAt` renders as fractional seconds (`1.70416464525e+09` form
  for fractional values; plain digits for whole seconds) after the
  code-review precision fix.
- Label keys mirror the `/stats` JSON field names (`build_date`,
  `go_version`, …); `dirty` renders `"true"`/`"false"`.
