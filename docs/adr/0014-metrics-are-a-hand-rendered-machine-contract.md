# Metrics are a hand-rendered machine contract

`GET /metrics` serves Prometheus-format **metrics** (ADR 0006's promised
endpoint) as text exposition 0.0.4, rendered directly from the same
snapshot `GET /stats` serves plus a scrape-time Go runtime sample — no
Prometheus client library, no registry, no collectors. The scalar-only,
one-snapshot output did not justify the dependency tree a client library
brings into `vendor/`; the renderer is a pure snapshot-to-text mapping,
pinned byte-exact by a golden test.

Consequences: metric names and types are a stable machine contract —
unlike `/stats`'s free shape, a series may be added but never renamed or
removed without a new ADR and a deprecation note in the metrics doc
(conservative Prometheus practice; scrapers fail silently on renames).
OpenMetrics is deferred: its stricter conformance buys nothing until
exemplars or native histograms are needed. Everything is sampled at
scrape time, so the response is one point-in-time snapshot; `go_memstats`
sampling briefly stops the world, accepted at scrape intervals.
