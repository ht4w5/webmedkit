# 03: Metrics contract doc, scrape config and alert examples

**What to build:** An operator wires monitoring from the docs alone, and
a maintainer can trust the docs cannot drift: a metrics contract
document — exposition format, the full family table with types and
meanings, counter-reset-on-restart semantics, the ADR 0014 evolution
policy (additions free, renames/removals breaking), and the
unauthenticated-listener exposure stance — whose byte-exact example is
pinned by a test against the same fixture the golden test uses (the
report-contract precedent). The document ships a ready-to-include
Prometheus scrape config and example alert rules for dropped-line growth
and tracker-cache eviction pressure, and the plan's HTTP API section
gains the endpoint without disturbing `/stats`'s non-contract status.

**Blocked by:** 02: Go runtime gauges on `/metrics`

**Status:** resolved

- [x] The contract document's byte-exact example is reproduced by a test
  (same fixture the golden test pins), so document and implementation
  cannot drift
- [x] Every family is listed with its type and meaning; counter resets on
  restart and the ADR 0014 evolution policy are stated explicitly
- [x] The scrape config snippet and example alert rules use exactly the
  documented metric names (dropped growth, eviction pressure near the
  entry cap)
- [x] The plan's HTTP API section documents `GET /metrics` and leaves
  `/stats`'s human-facing, non-contract status unchanged
- [x] The unauthenticated exposure of the endpoint is called out for
  operators

## Answer

Implemented on integration branch `feat/prometheus-metrics` (merge
commit `7277560`; cross-ticket code-review fixes through `7fa5a22`).
`docs/metrics.md` is the metrics contract beside `docs/reports.md` and
matching its shape: exposition format, the full family table (every
family's type, meaning and labels — rows pinned against
`metricFamilies` by `TestMetricsDocumentTableMatchesMetricFamilies`),
counter-reset-on-restart semantics, the ADR 0014 evolution policy, the
unauthenticated-listener exposure stance (GET-only, 405 otherwise), a
ready-to-include scrape config and documentation-only alert rules
(`BandageDroppedLogLines`, `BandageTrackerCacheEvictionPressure` at 90%
of the entry cap) on exactly the contracted names. The document's
byte-exact example is reproduced by
`TestMetricsDocumentMatchesDocsExample` against the same
`metricsGoldenResponse` the golden test pins through the wire (the
report-contract precedent). `PLAN.md` §7 gained the `GET /metrics`
bullet; `/stats`'s human-facing, non-contract status is untouched.

Context: spec `../spec.md`, `docs/metrics.md`,
`docs/adr/0014-metrics-are-a-hand-rendered-machine-contract.md`,
`docs/adr/0006-human-facing-stats-prometheus-for-metrics.md`.

Notes:
- The doc carries exactly one fenced `text` block (the pinned example);
  scrape config and alert rules use `yaml` fences, so extraction is
  unambiguous.
- No alert-rule files shipped and no CI runs them — documentation only,
  per the spec's Out of Scope.
