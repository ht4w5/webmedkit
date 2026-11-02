# 03: bytedance/sonic for nginx log parsing

**What to build:** The nginx log-parse path decodes line JSON with
bytedance/sonic instead of the standard library decoder, raising ingest
throughput on high-rate access logs. The swap is behavior-preserving: the
same line shape decodes into the same parsed Event and nothing else in the
codebase changes its JSON behavior. The dependency is vendored like the
tree's existing third-party dependencies; amd64 is the deployment target.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The nginx parse path decodes JSON via sonic; no other JSON call sites change.
- [x] The existing nginx parser tests pass unchanged - they are the proof of behavior preservation.
- [x] Benchmarks over the shared corpus run and show the decoder swap is at worst neutral, ideally faster.
- [x] The dependency is vendored and the tree builds and tests clean (`make build`, `make test`).
- [x] The "sonic for nginx log parsing" ADR records the trade-off against the standard library decoder.

## Answer

Implemented on `ticket/wam-03-sonic-nginx-json` (commit `c233339`),
merged into `integration/web-panel-api-move` (`ee1a0c4`). The nginx
line decode uses bytedance/sonic (`lineDecoder`, pinned
`CaseSensitive`+`ValidateString` to the historic contract) with the
same `rawFields`/`fieldValue` capture shape; duplicate names stay
last-wins, invalid UTF-8 still mangles per-byte to U+FFFD, and the
drop-log error text is replayed through encoding/json v1 so it stays
byte-identical. The existing nginx parser tests pass UNCHANGED (zero
test files in the diff). Benchmarks over the shared corpus
(`bench.NewCorpus`, `-count=10`, benchstat) show the swap is faster:
minimal -19.1% sec/op (+23.7% lines/s), full -26.4% (+35.9% lines/s),
at the honest cost of +2 allocs/op and +37-62% B/op. Vendored like the
tree's other third-party deps; `make build`/`make test` clean. ADR
`docs/adr/0023-sonic-decodes-nginx-log-lines.md` records the trade-off
against the standard library decoder. Reviewed in `62e3efa` (lineDecoder
rename).
