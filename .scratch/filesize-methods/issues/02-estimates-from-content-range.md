# 02: Estimates from Content-Range totals

**What to build:** The second observation source: the bandage nginx log
format carries `Content-Range`, the Event carries the parsed total, and
a GET 206 observes the file's exact total - so a range transfer counts
as its true fraction of a download toward replay, and a seeked or
resumed download totals exactly one download-equivalent instead of one
per request. Exotic `Content-Range` values degrade to "no observation"
and never drop the event. Demo: a resumable download across several
range transfers accumulates its true fraction of a download-equivalent.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] The bandage nginx log format carries the response `Content-Range` as `crange`; old flux-format lines without it keep parsing unchanged
- [ ] `bytes <a>-<b>/<total>` and `bytes */<total>` yield the total on the Event; `*` totals and unparsable values yield "no size information" - the line is kept and counted as normal traffic, never dropped
- [ ] GET 206 with a usable total is an observation source (the Sampling rule); `416`, non-206 statuses and headerless `206` are not
- [ ] A `206` without a usable total is enriched with the current estimate and adds no observation
- [ ] Header syntax never reaches the domain: the parser extracts the total
- [ ] End-to-end: a range/resumed download accumulates its true download-equivalent fraction across requests; replay volumes derive from the exact total

Context: spec `../spec.md` (Implementation Decisions 1, 3, 5).
