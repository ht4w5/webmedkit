# 02: Go runtime gauges on `/metrics`

**What to build:** The operator's scrape response additionally carries
the Go runtime block — `go_info{version}`, `go_goroutines`, `go_threads`,
`go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`,
`go_memstats_sys_bytes`, `go_memstats_gc_cpu_fraction` — read at scrape
time so each response stays one point-in-time snapshot. The runtime
sample sits behind a narrow port beside its consumer, in the same style
as the existing stats source port (ADR 0005), satisfied by a
standard-library-backed adapter, so tests inject fixed values and the
HTTP surface stays dependency-free.

**Blocked by:** 01: Scrapeable metrics endpoint (stats mirror)

**Status:** resolved

- [x] The runtime families appear in the same `/metrics` response, with
  `# HELP`/`# TYPE`, conventional `go_` names, and the curated subset
  only (no full memstats surface)
- [x] Runtime state is read only at scrape time: no background sampler,
  no cached values between scrapes
- [x] A narrow runtime source port sits beside its consumer in the same
  style as the stats source port; a standard-library adapter satisfies
  it (thread count from the thread-create profile count)
- [x] Unit tests pin the runtime block through injected fixed values,
  folded into the golden fixture; a live-daemon test asserts the block by
  shape only, since real values are non-deterministic
- [x] No new dependencies

## Answer

Implemented on integration branch `feat/prometheus-metrics` (merge
commit `4eaad24`; cross-ticket code-review fixes through `7fa5a22`).
The curated runtime block — `go_info{version}`, `go_goroutines`,
`go_threads`, `go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`,
`go_memstats_sys_bytes`, `go_memstats_gc_cpu_fraction` — rides the same
`/metrics` response behind the new narrow `RuntimeSource` port beside
its consumer in `httpapi` (ADR 0005 discipline), satisfied by the
stdlib `goRuntime` adapter in `app` (thread count from the thread-create
profile count). Everything is read fresh per scrape: no sampler, no
caching; the renderer stays pure. Fixed injected runtime values fold
into the golden fixture; a live-daemon e2e test asserts the block by
shape only.

Context: spec `../spec.md`, `docs/adr/0014-metrics-are-a-hand-rendered-machine-contract.md`,
`docs/adr/0005-logging-zerolog-behind-domain-port.md`.

Notes:
- `New(reports, stats, runtime, now)` carries the port; call sites
  `daemon.go` (real adapter) and `httpapi_bench_test.go` (fake).
- `go_memstats_gc_cpu_fraction` renders in shortest `'g'` float form
  (`0.0125`; live values may appear as `1.5e-05`).
