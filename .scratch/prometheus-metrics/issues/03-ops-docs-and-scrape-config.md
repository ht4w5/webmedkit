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

**Status:** ready-for-agent

- [ ] The contract document's byte-exact example is reproduced by a test
  (same fixture the golden test pins), so document and implementation
  cannot drift
- [ ] Every family is listed with its type and meaning; counter resets on
  restart and the ADR 0014 evolution policy are stated explicitly
- [ ] The scrape config snippet and example alert rules use exactly the
  documented metric names (dropped growth, eviction pressure near the
  entry cap)
- [ ] The plan's HTTP API section documents `GET /metrics` and leaves
  `/stats`'s human-facing, non-contract status unchanged
- [ ] The unauthenticated exposure of the endpoint is called out for
  operators
