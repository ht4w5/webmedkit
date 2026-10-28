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

**Status:** ready-for-agent

- [ ] The runtime families appear in the same `/metrics` response, with
  `# HELP`/`# TYPE`, conventional `go_` names, and the curated subset
  only (no full memstats surface)
- [ ] Runtime state is read only at scrape time: no background sampler,
  no cached values between scrapes
- [ ] A narrow runtime source port sits beside its consumer in the same
  style as the stats source port; a standard-library adapter satisfies
  it (thread count from the thread-create profile count)
- [ ] Unit tests pin the runtime block through injected fixed values,
  folded into the golden fixture; a live-daemon test asserts the block by
  shape only, since real values are non-deterministic
- [ ] No new dependencies
