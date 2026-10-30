# Prometheus metrics for service health - spec

Status: ready-for-agent

## Problem Statement

Operators running bandage cannot monitor service health with standard
tooling. The only ops surfaces are `GET /ping` (liveness) and `GET /stats`
- the human-facing **Statistic** snapshot, indented JSON with no format
contract (ADR 0006), explicitly not scrapeable. Alerting on dropped
lines, tracker-cache eviction pressure, or a stalled pipeline means
reverse-engineering that JSON shape and breaking on every field addition.
ADR 0006 promised a separate Prometheus-format endpoint "if
machine-readable metric gathering is ever needed" - it is: without
**Metrics**, bandage is the one service in the fleet an operator cannot
dashboard or alert on.

## Solution

`GET /metrics` on the daemon's existing HTTP listener serves Prometheus
text exposition 0.0.4: a `bandage_`-prefixed metric family per statistic
in the same cumulative-since-start snapshot `GET /stats` serves, plus a
scrape-time Go runtime sample. Metric names and types are a stable
machine contract (ADR 0014): series may be added, never silently renamed
or removed. The exposition text is rendered by hand from a plain snapshot
- no client library - and `/stats` stays exactly as it is for human eyes.

## User Stories

1. As an operator, I want a Prometheus-format metrics endpoint on the
   daemon's existing HTTP listener, so that I can monitor service health
   with the same scraping and alerting tools as every other service.
2. As an operator, I want the endpoint always on with no configuration
   knob, so that monitoring works out of the box like `/ping` and
   `/stats`.
3. As an operator, I want machine-gathered metrics separate from the
   human-facing Statistic at `/stats`, so that neither surface's contract
   constrains the other (ADR 0006).
4. As an operator, I want the endpoint on the same unauthenticated
   listener as the rest of the API, so that deployment wiring stays
   unchanged.
5. As an operator, I want cumulative-since-start counters for received,
   parsed and dropped log lines, so that I can see pipeline throughput
   and loss at a glance.
6. As an operator, I want the dropped counter to mean exactly what the
   `/stats` dropped statistic means, so that the two views can never
   disagree about the same daemon.
7. As an alert author, I want a dropped-lines counter, so that I can
   alert the moment the pipeline starts losing events.
8. As an alert author, I want tracker-cache gets, sets, misses and
   evictions as counters, so that I can detect cache thrash before it
   degrades analysis.
9. As an alert author, I want tracker-cache entries and the entry cap as
   gauges, so that I can alert on eviction pressure as the cache
   approaches its cap.
10. As an alert author, I want goroutine count as a gauge, so that I can
    alert on leaks across deployments.
11. As a dashboard author, I want a start-time gauge, so that uptime is
    computable as `time() - start` without maintaining a separate uptime
    series.
12. As a dashboard author, I want the worker count as a gauge, so that
    throughput panels can be normalized per worker.
13. As a dashboard author, I want a build-info gauge carrying the build
    identity as labels, so that I can join version, commit and platform
    into every panel and bisect incidents by build.
14. As a dashboard author, I want Go runtime series under their
    conventional `go_` names, so that existing generic Go dashboards work
    unchanged.
15. As a scraper, I want every metric family preceded by `# HELP` and
    `# TYPE` lines, so that I parse the endpoint without special cases.
16. As a scraper, I want the response content type to declare the
    exposition format version, so that I recognize the format without
    guessing.
17. As a scraper, I want label values escaped per the exposition format,
    so that an unusual commit string cannot corrupt the response.
18. As an operator, I want counters cumulative since daemon start with
    resets on restart, so that `rate()` handles restarts correctly and a
    counter reset always means "the daemon restarted".
19. As an operator, I want every scrape response to be one point-in-time
    snapshot, so that families within a single response never mix
    sampling times.
20. As an operator, I want metric names and types to be a stable machine
    contract - series added freely, never renamed or removed (ADR 0014)
    - so that dashboards and alert rules survive upgrades.
21. As an operator, I want Go runtime series for goroutines, threads,
    allocation, heap in use, system memory, GC CPU fraction and Go
    version, so that I can diagnose resource problems without attaching a
    debugger.
22. As an operator, I want the runtime block to stay a curated subset, so
    that the endpoint stays readable and the contract small.
23. As an operator, I want the human-facing `/stats` to stay exactly as
    it is, so that existing ops habits and ADR 0006 remain true.
24. As an operator, I want the endpoint's unauthenticated exposure
    documented, so that I can decide at the network layer what to expose.
25. As a maintainer, I want the exposition rendered from a plain snapshot
    by a pure function, so that output is deterministic and testable
    without running a daemon.
26. As a maintainer, I want each family's name, type, help text and
    source single-sourced in one table, so that adding a series touches
    exactly one place.
27. As a maintainer, I want the exposition hand-rendered without a
    Prometheus client library, so that the dependency tree and vendor
    footprint stay as they are (ADR 0014).
28. As a maintainer, I want the runtime sample behind a narrow port
    beside its consumer, mirroring the existing stats source port, so
    that tests inject fixed values and the HTTP module depends only on
    the standard library.
29. As a maintainer, I want runtime state read only at scrape time, so
    that the feature introduces no background goroutine and no staleness
    policy to reason about.
30. As a maintainer, I want tests that assert only the bytes on the wire,
    so that the suite survives renderer refactors.
31. As a metrics consumer, I want the metrics contract document to carry
    a byte-exact example pinned by a test, so that document and
    implementation cannot drift - the same guarantee the report contract
    gives.
32. As an operator, I want the metrics document to ship a scrape config
    and example alert rules, so that I wire monitoring without reading
    the code.

## Implementation Decisions

- **Endpoint**: `GET /metrics` on the existing read-only HTTP surface,
  registered beside `/ping` and `/stats`; no URL version; GET only;
  always registered.
- **Rendering model**: the handler assembles one metrics snapshot - the
  statistics block `/stats` already serves plus a runtime block - and
  renders it through a pure snapshot-to-text renderer. The renderer never
  reads live state; determinism is what makes the output golden-testable.
- **Format**: classic Prometheus text exposition 0.0.4, not OpenMetrics
  (deferred per ADR 0014 until exemplars or native histograms are
  needed). Content type `text/plain; version=0.0.4; charset=utf-8`; every
  family preceded by `# HELP` and `# TYPE`; label values escaped per the
  exposition spec (backslash, quote, newline); lines newline-terminated;
  families in a fixed deterministic order.
- **Naming**: `bandage_` prefix - the binary is what gets scraped, not
  the repository name. `_total` suffix on counters; gauges bare. Metric
  names and types are the stable contract of ADR 0014.
- **Metric families** (stats mirror; every value already in the snapshot
  `/stats` serves):
  - `bandage_start_time_seconds` (gauge) - daemon start time, for
    uptime as `time() - start`.
  - `bandage_build_info` (gauge, value 1) - labels carry the full build
    identity: name, version, commit, build date, dirty, Go version,
    platform. Cardinality is 1; labels are for joining and bisection.
  - `bandage_pipeline_events_received_total`,
    `bandage_pipeline_events_parsed_total`,
    `bandage_pipeline_events_dropped_total` (counters).
  - `bandage_pipeline_workers` (gauge).
  - `bandage_tracker_cache_gets_total`, `_sets_total`, `_misses_total`,
    `_evictions_total` (counters).
  - `bandage_tracker_cache_entries`, `bandage_tracker_cache_entry_cap`
    (gauges).
- **Runtime block** (standard library only), conventional `go_` names:
  `go_info{version}` (gauge 1), `go_goroutines`, `go_threads`,
  `go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`,
  `go_memstats_sys_bytes`, `go_memstats_gc_cpu_fraction`. The full
  memstats surface is deliberately not exposed; additions are free under
  the contract.
- **Ports**: the existing narrow stats source port stays as is. A new
  narrow runtime source port sits beside its consumer in the same style
  (ports beside consumers, per ADR 0005's discipline), satisfied by a
  standard-library-backed adapter. One behavior seam total.
- **Sampling**: everything is read at scrape time - atomic counter reads
  and the runtime memory sample alike - so the response is one
  point-in-time snapshot; the brief stop-the-world of the memory sample
  is accepted at normal scrape intervals. No background sampler, no
  caching between scrapes.
- **Counter semantics**: cumulative since start; a restart resets them.
  This is expected Prometheus behavior and is stated in the contract
  document.
- **`/stats` untouched**: both surfaces render the same snapshot source;
  neither changes the other's contract (ADR 0006 stands).
- **Contract document**: a metrics document published beside the report
  contract, carrying the format, the family table, counter semantics and
  a byte-exact example; the plan's HTTP API section gains the endpoint.
  Evolution policy per ADR 0014: additions free, renames and removals
  are breaking and require a new ADR plus a deprecation note.

## Testing Decisions

- A good test asserts external behavior only: the bytes a `GET /metrics`
  response carries. No renderer-internals assertions, no registry-style
  introspection - the renderer is a pure function precisely so the wire
  bytes are the whole observable surface.
- **Seams**: the highest seam is the HTTP endpoint itself, driven with
  injected fixed snapshots. The existing stats source port is preferred
  and reused (already the daemon's structural seam); the one new seam is
  the runtime source port, proposed at the highest point possible - the
  same handler constructor - so tests pin runtime values too. No seams
  below the handler.
- Prior art: the report-document test that reproduces the contract's own
  example byte-for-byte; the shipped-format tests that pin the shipped
  configs; the daemon end-to-end tests that drive the HTTP surface.
- Cases: one golden full response over fixed injected snapshots
  (byte-exact, family order and all); a nasty label value case (commit
  string with quotes, backslashes, newline) proving escaping; `# HELP`
  and `# TYPE` present for every family; the content-type header; non-GET
  rejected; under the live daemon, the runtime block asserted by shape
  only, since real values are inherently non-deterministic.

## Out of Scope

- New domain counters (reports written, trigger suppressions, sweep
  deletions, escalation overrides, parse errors) - the stats mirror only;
  the contract makes later additions free.
- OpenMetrics, exemplars, histograms, a push gateway, remote write.
- A Prometheus client library, registry, or collectors.
- A separate listener, authentication, or TLS for the endpoint - the
  unauthenticated-listener stance of ADR 0006 carries over unchanged.
- Any configuration knob (enable/disable, path, label sets).
- Any change to `/stats`, its shape, or its non-contract status.
- The full `go_memstats` surface, pprof endpoints, or per-Target /
  per-rule labeled series (cardinality is deliberately never a function
  of traffic).
- Shipped alert-rule files exercised in CI - the docs carry example alert
  rules as documentation only.

## Further Notes

- This realizes the endpoint ADR 0006 reserved; ADR 0014 records the two
  new decisions taken here (hand-rendered exposition, stable name
  contract) and the OpenMetrics deferral.
- The glossary's Operations terms distinguish the two surfaces:
  **Statistic** (human snapshot, no contract) and **Metric**
  (machine-contract series).
- Every metric's meaning is exactly its `/stats` counterpart's; where the
  two are read side by side they must agree by construction, since one
  snapshot feeds both.
- The endpoint's exposure stance is the one ADR 0006 already accepted:
  a read-only ops surface on the unauthenticated listener, safe to
  firewall at the network layer.
