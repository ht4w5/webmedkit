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

**Status:** ready-for-agent

- [ ] `GET /metrics` on the existing listener serves the families:
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
- [ ] Every family is preceded by `# HELP` and `# TYPE`; families appear
  in a fixed deterministic order; label values are escaped per the
  exposition spec (quotes, backslashes, newlines)
- [ ] The response content type declares the exposition format version
  (`text/plain; version=0.0.4; charset=utf-8`); non-GET methods are
  rejected
- [ ] Metric values agree with the `/stats` snapshot by construction —
  one snapshot feeds both surfaces
- [ ] Counters are cumulative since start; the renderer depends only on
  the standard library (no new dependencies)
- [ ] Tests at the wire seam: a golden byte-exact full response over
  fixed injected snapshots (family order included), a nasty-label
  escaping case, and an endpoint round-trip asserting the content type
