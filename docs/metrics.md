# The metrics contract

`GET /metrics` serves the daemon's machine-contract **Metrics** - the
endpoint [ADR 0006](adr/0006-human-facing-stats-prometheus-for-metrics.md)
reserved, realized under
[ADR 0014](adr/0014-metrics-are-a-hand-rendered-machine-contract.md).
This document is the contract a scraper, dashboard or alert rule
integrates against instead of reverse-engineering the code: the
exposition format, every metric family with its type and meaning, the
counter semantics, the evolution policy, the exposure stance, and a
ready-to-include scrape config with example alert rules. Its example is
reproduced byte-for-byte by a test
(`bandage/internal/infra/httpapi/metricsdoc_test.go`), so this contract
and the implementation cannot drift.

`GET /api/stats` remains the human-facing **Stats** snapshot: the same
cumulative-since-start snapshot for operator eyes, unversioned and with
no format contract ([ADR 0006](adr/0006-human-facing-stats-prometheus-for-metrics.md)).
Neither surface's contract constrains the other. Each endpoint takes
its own point-in-time snapshot per request, so every response is
internally consistent - but two separate requests, one per surface, are
two snapshots and can disagree.

## Exposition format

The response is classic Prometheus text exposition 0.0.4 - not
OpenMetrics, deferred per
[ADR 0014](adr/0014-metrics-are-a-hand-rendered-machine-contract.md)
until exemplars or native histograms are needed. The content type
declares the format version:
`text/plain; version=0.0.4; charset=utf-8`.

Every family is preceded by `# HELP` and `# TYPE` lines; label values
are escaped per the exposition format (backslash, double quote,
newline); every line is newline-terminated; families appear in a fixed
deterministic order. The exposition is rendered by hand from a plain
snapshot - no Prometheus client library
([ADR 0014](adr/0014-metrics-are-a-hand-rendered-machine-contract.md)).
Each response is one point-in-time snapshot: the `/api/stats` snapshot block and
the runtime block are alike read at scrape time, so families within a
single response never mix sampling times.

## Metric families

The full contract, in exposition order. The meaning column is the
family's pinned `# HELP` text. Counters carry the `_total` suffix;
gauges are bare.

| family | type | meaning |
|---|---|---|
| `bandage_start_time_seconds` | gauge | Seconds since the Unix epoch at which the daemon started. |
| `bandage_build_info` | gauge | Build identity of the running binary; the labels carry the identity and the value is always 1. |
| `bandage_pipeline_events_received_total` | counter | Log lines received from the wire since daemon start. |
| `bandage_pipeline_events_parsed_total` | counter | Log lines parsed into events since daemon start. |
| `bandage_pipeline_events_dropped_total` | counter | Malformed or invalid log lines dropped since daemon start. |
| `bandage_pipeline_workers` | gauge | Pipeline workers processing log lines. |
| `bandage_tracker_cache_gets_total` | counter | Tracker cache lookups since daemon start. |
| `bandage_tracker_cache_sets_total` | counter | Tracker cache writes since daemon start. |
| `bandage_tracker_cache_misses_total` | counter | Tracker cache lookups that found no entry since daemon start. |
| `bandage_tracker_cache_evictions_total` | counter | Tracker cache evictions since daemon start. |
| `bandage_tracker_cache_entries` | gauge | Tracker cache entries currently held. |
| `bandage_tracker_cache_entry_cap` | gauge | Configured tracker cache entry capacity. |
| `bandage_rule_events_total` | counter | Events evaluated against the rule since daemon start. |
| `bandage_rule_events_matched_total` | counter | Events that matched the rule since daemon start. |
| `bandage_enrichment_runs_total` | counter | Enrichment runs for the rule and enrichment since daemon start. |
| `bandage_enrichment_runs_observed_total` | counter | Enrichment runs that fed the enrichment's learned state since daemon start. |
| `bandage_enrichment_runs_enriched_total` | counter | Enrichment runs that produced a non-default derived value since daemon start. |
| `go_info` | gauge | Information about the Go environment. |
| `go_goroutines` | gauge | Number of goroutines that currently exist. |
| `go_threads` | gauge | Number of OS threads created. |
| `go_memstats_alloc_bytes` | gauge | Number of bytes allocated and still in use. |
| `go_memstats_heap_inuse_bytes` | gauge | Number of heap bytes that are in use. |
| `go_memstats_sys_bytes` | gauge | Number of bytes obtained from system. |
| `go_memstats_gc_cpu_fraction` | gauge | The fraction of this program's available CPU time used by the GC since the program started. |

Seven families carry labels. `bandage_build_info` carries the build
identity as labels - `name`, `version`, `commit`, `build_date`,
`dirty`, `go_version`, `platform` - so panels can join version, commit
and platform into every series and incidents can be bisected by build;
its value is always `1`. `go_info` carries one label, `version`. The
two rule counter families each carry one label, `rule`, holding the
rule ID: `bandage_rule_events_total` counts the events evaluated
against the rule since daemon start, `bandage_rule_events_matched_total`
the subset that matched it. The three enrichment counter families
each carry two labels, `rule` and `enrichment`, holding the (rule,
enrichment) pair: `bandage_enrichment_runs_total` counts the pair's
enrichment runs since daemon start,
`bandage_enrichment_runs_observed_total` the subset that fed the
enrichment's learned state,
`bandage_enrichment_runs_enriched_total` the subset that produced a
non-default derived value. Cardinality is fixed per configuration: no
family's series count is ever a function of traffic - the rule families'
series count is the number of counters enabled in the rules file, the
enrichment families' the number of counter-enabled (rule, enrichment)
pairs, and every enabled rule and pair emits its series from daemon
start with zeros, so `rate()` and `increase()` work before the first
match.

The enrichment families carry the **Enrichment counter**
([GLOSSARY.md](../GLOSSARY.md)) per (rule, enrichment) pair; its three
quantities are fixed for every kind, meanings defined per kind. The
Filesize enrichment's `observed` counts the runs that fed the estimate
- its Sampling-rule-eligible responses ([GLOSSARY.md](../GLOSSARY.md))
- and its `enriched` the runs that produced a non-default derived
value. A kind without learned state reports `observed` equal to
`total`, never an omitted quantity.

The `bandage_` families mirror their **Stats** counterparts field
for field - a dropped line is the same event on both surfaces. For the
rule families the mirror is fields and values, not ordering: their
series render in rule-ID order while the **Stats** rules block lists
rules-file order. For the enrichment families the mirror is fields,
values and ordering: their series render in rules-file order of pairs,
exactly as the **Stats** enrichment block lists them. The asymmetry
between the two counter family groups is deliberate and pinned - rule
series in rule-ID order, enrichment series in rules-file order of
pairs - so both orderings are contract a scraper may rely on. The
`go_` families are a curated subset of Go runtime state under the
conventional `go_` names, so generic Go dashboards work unchanged; the
full memstats surface is deliberately not exposed.

## Counters and restarts

Counters are cumulative since daemon start. A daemon restart resets
them - expected Prometheus behavior, handled transparently by `rate()`
and `increase()` - so a counter reset always means "the daemon
restarted", and `bandage_start_time_seconds` says exactly when. Gauges
are instantaneous readings at scrape time.

## Evolution policy

Metric names and types are a stable machine contract
([ADR 0014](adr/0014-metrics-are-a-hand-rendered-machine-contract.md)):
a series may be added freely, but a family is never silently renamed or
removed - scrapers fail silently on renames. A rename or removal is a
breaking change and requires a new ADR plus a deprecation note in this
document. Consumers must therefore tolerate unknown families; dashboards
and alert rules that name families from this contract survive upgrades.

## Exposure

`GET /metrics` is a read-only ops surface on the same unauthenticated
listener as the rest of the daemon's web surface: `/ping`, the Web Panel
at `GET /` (with its embedded assets), and the machine API under
`/api/` - `/api/stats`, and the report API's `GET /api/reports` and
`DELETE /api/reports` by rule and target. No authentication, no TLS, no
configuration knob, always registered. This is the exposure
stance [ADR 0006](adr/0006-human-facing-stats-prometheus-for-metrics.md)
already accepted, carried over unchanged. The endpoint is GET only:
HEAD - which some scrapers and proxies send in place of GET - and
every other method are rejected with `405 Method Not Allowed` and an
`Allow: GET` header. The daemon serves the endpoint; deciding who may reach it is the operator's network-layer
job - firewall the listener so only the monitoring network can scrape
it, as with the rest of the ops surface.

## Scraping

The endpoint needs no preparation beyond reachability. A scrape
configuration to include as-is, with the target adjusted to the
daemon's `api.listen` address:

```yaml
scrape_configs:
  - job_name: bandage
    metrics_path: /metrics
    scrape_interval: 15s
    static_configs:
      - targets: ["127.0.0.1:8080"]
```

### Example alert rules

The rules below are documentation examples - the daemon ships no alert
rule files. Two starting points: dropped-line growth (the pipeline
losing events) and tracker-cache eviction pressure (entries approaching
the entry cap). Both use exactly the metric names contracted above.

```yaml
groups:
  - name: bandage
    rules:
      - alert: BandageDroppedLogLines
        expr: increase(bandage_pipeline_events_dropped_total[5m]) > 0
        labels:
          severity: warning
        annotations:
          summary: bandage is dropping log lines
          description: "{{ $value }} log lines dropped in the last 5 minutes."
      - alert: BandageTrackerCacheEvictionPressure
        expr: bandage_tracker_cache_entries > 0.9 * bandage_tracker_cache_entry_cap
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: tracker cache is nearing its entry cap
          description: "entries at {{ $value }} and climbing toward the cap; evictions follow."
```

`BandageDroppedLogLines` fires on the first scrape that shows the
dropped counter growing - the moment the pipeline starts losing events;
`increase()` accounts for the counter resets described above, so a
restart does not fire it. `BandageTrackerCacheEvictionPressure` fires
when `bandage_tracker_cache_entries` sits within 10% of
`bandage_tracker_cache_entry_cap` for ten minutes: eviction pressure,
the cache about to evict what it holds. Tune the window, the ratio and
the severities to taste.

## Example

A complete `GET /metrics` response over a fixed sample snapshot -
family order and every byte:

```text
# HELP bandage_start_time_seconds Seconds since the Unix epoch at which the daemon started.
# TYPE bandage_start_time_seconds gauge
bandage_start_time_seconds 1.70416464525e+09
# HELP bandage_build_info Build identity of the running binary; the labels carry the identity and the value is always 1.
# TYPE bandage_build_info gauge
bandage_build_info{name="bandage",version="1.2.3",commit="abc123",build_date="2024-01-02T03:04:05Z",dirty="false",go_version="go1.27.0",platform="linux/amd64"} 1
# HELP bandage_pipeline_events_received_total Log lines received from the wire since daemon start.
# TYPE bandage_pipeline_events_received_total counter
bandage_pipeline_events_received_total 1000
# HELP bandage_pipeline_events_parsed_total Log lines parsed into events since daemon start.
# TYPE bandage_pipeline_events_parsed_total counter
bandage_pipeline_events_parsed_total 990
# HELP bandage_pipeline_events_dropped_total Malformed or invalid log lines dropped since daemon start.
# TYPE bandage_pipeline_events_dropped_total counter
bandage_pipeline_events_dropped_total 10
# HELP bandage_pipeline_workers Pipeline workers processing log lines.
# TYPE bandage_pipeline_workers gauge
bandage_pipeline_workers 4
# HELP bandage_tracker_cache_gets_total Tracker cache lookups since daemon start.
# TYPE bandage_tracker_cache_gets_total counter
bandage_tracker_cache_gets_total 800
# HELP bandage_tracker_cache_sets_total Tracker cache writes since daemon start.
# TYPE bandage_tracker_cache_sets_total counter
bandage_tracker_cache_sets_total 400
# HELP bandage_tracker_cache_misses_total Tracker cache lookups that found no entry since daemon start.
# TYPE bandage_tracker_cache_misses_total counter
bandage_tracker_cache_misses_total 200
# HELP bandage_tracker_cache_evictions_total Tracker cache evictions since daemon start.
# TYPE bandage_tracker_cache_evictions_total counter
bandage_tracker_cache_evictions_total 5
# HELP bandage_tracker_cache_entries Tracker cache entries currently held.
# TYPE bandage_tracker_cache_entries gauge
bandage_tracker_cache_entries 350
# HELP bandage_tracker_cache_entry_cap Configured tracker cache entry capacity.
# TYPE bandage_tracker_cache_entry_cap gauge
bandage_tracker_cache_entry_cap 1024
# HELP bandage_rule_events_total Events evaluated against the rule since daemon start.
# TYPE bandage_rule_events_total counter
bandage_rule_events_total{rule="alpha"} 0
bandage_rule_events_total{rule="zeta"} 12
# HELP bandage_rule_events_matched_total Events that matched the rule since daemon start.
# TYPE bandage_rule_events_matched_total counter
bandage_rule_events_matched_total{rule="alpha"} 0
bandage_rule_events_matched_total{rule="zeta"} 3
# HELP bandage_enrichment_runs_total Enrichment runs for the rule and enrichment since daemon start.
# TYPE bandage_enrichment_runs_total counter
bandage_enrichment_runs_total{rule="zeta",enrichment="filesize"} 7
bandage_enrichment_runs_total{rule="alpha",enrichment="filesize"} 0
# HELP bandage_enrichment_runs_observed_total Enrichment runs that fed the enrichment's learned state since daemon start.
# TYPE bandage_enrichment_runs_observed_total counter
bandage_enrichment_runs_observed_total{rule="zeta",enrichment="filesize"} 5
bandage_enrichment_runs_observed_total{rule="alpha",enrichment="filesize"} 0
# HELP bandage_enrichment_runs_enriched_total Enrichment runs that produced a non-default derived value since daemon start.
# TYPE bandage_enrichment_runs_enriched_total counter
bandage_enrichment_runs_enriched_total{rule="zeta",enrichment="filesize"} 2
bandage_enrichment_runs_enriched_total{rule="alpha",enrichment="filesize"} 0
# HELP go_info Information about the Go environment.
# TYPE go_info gauge
go_info{version="go1.27.0"} 1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 7
# HELP go_threads Number of OS threads created.
# TYPE go_threads gauge
go_threads 5
# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 1234567
# HELP go_memstats_heap_inuse_bytes Number of heap bytes that are in use.
# TYPE go_memstats_heap_inuse_bytes gauge
go_memstats_heap_inuse_bytes 2345678
# HELP go_memstats_sys_bytes Number of bytes obtained from system.
# TYPE go_memstats_sys_bytes gauge
go_memstats_sys_bytes 3456789
# HELP go_memstats_gc_cpu_fraction The fraction of this program's available CPU time used by the GC since the program started.
# TYPE go_memstats_gc_cpu_fraction gauge
go_memstats_gc_cpu_fraction 0.0125
```

The first fenced `text` block of this document is the canonical
example; the test extracts it and compares it byte for byte with the
`GET /metrics` body over the fixture the golden test pins (the response
carries one trailing newline the fence does not).
