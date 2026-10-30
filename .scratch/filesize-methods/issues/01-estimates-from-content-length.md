# 01: Estimates from Content-Length

**What to build:** The Filesize estimate learns from response sizes
instead of transferred bytes, starting with the first source: the
bandage nginx log format carries each response's `Content-Length`, the
Event carries it as a parsed response length, and the estimate (the
existing EWMA and warm-up gate) observes it only for GET/HEAD 200
responses that carry one - the Sampling rule. Everything that used to
drag the estimate down (304 revalidations, length-less HEADs, error
bodies, partial transfers) stops moving it. A malformed `clen` field
drops and counts the line like any other invalid field. Demo: feed
access-log lines and watch the derived size track the file's length;
replay tolerance stops shrinking on ordinary cached browsing.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The bandage nginx log format carries the response `Content-Length` as `clen`; old flux-format lines without it keep parsing unchanged
- [ ] Events carry the parsed response length; absent, empty or zero means "no size information"
- [ ] A negative or non-numeric `clen` is an invalid field: the line is dropped and counted, like `sent`
- [ ] Observations come only from GET/HEAD 200 events with a response length (the Sampling rule) - never from transferred bytes, `304`, length-less `HEAD`, `206`, or error statuses, whatever else admits an event
- [ ] Events that are not observation-eligible (e.g. chunked responses without a length) are enriched with the current estimate and add no observation, so chunked downloads stay inside replay accounting
- [ ] The EWMA math, exact seeding and the warm-up gate are unchanged - the enricher's existing table tests pass re-pinned to response-size observations
- [ ] End-to-end: raw log lines through parse, enrich and evaluate yield the expected derived sizes and replay outcomes; pipeline benchmarks show no regression

Context: spec `../spec.md` (Implementation Decisions 1, 3-4).
