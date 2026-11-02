# Web panel for report review, API move, and production tuning - spec

Status: ready-for-agent

## Problem Statement

Operators running bandage have no way to review **Reports** without shell
tooling: the report document is served as raw JSON on the same path that
machine consumers fetch, so "look at what matched and clean up stale
reports" means curl plus jq plus a separate DELETE call whose enforcement
consequences are invisible. There is no human-facing surface at the web
root at all, and the machine API sits at bare paths that any future page
would shadow.

Meanwhile production operation is noisy and slow in two places: tracker
cache **Eviction** is steady-state in production yet logs at warn level,
flooding the log; and the nginx log parser's JSON decoding leaves
throughput on the table at high ingest rates. Finally, the **Filter**
DSL can match paths and user agents only by prefix, suffix, or keyword -
rules authors cannot express patterns like extension families or bot
families without stacking brittle keyword clauses.

## Solution

A single embedded web page at the web root gives operators a dashboard:
a read-only Statistic view and a report-review table where each **Report**
can be inspected (full body: **Reason**, **Penalty**, **Sample**, expiry)
and deleted, with the delete confirmation stating plainly that the
enforcement derived from the report will be withdrawn at the enforcing
consumer's next sync. A regex filter box narrows the list client-side.
The machine API moves under `/api/` so page routes and API routes never
collide; `/metrics` and `/ping` stay where scrapers and probes expect
them. Eviction logging drops to debug level. The filter DSL gains `path
regex` and `agent regex` matchers. The nginx log parser decodes JSON with
bytedance/sonic instead of the standard library.

## User Stories

1. As an operator, I want a web panel served at the web root, so that I
   can review reports from a browser without assembling curl and jq
   commands.
2. As an operator, I want a Stats tab showing the **Statistic**
   snapshot, so that I can see daemon health at a glance next to the
   reports it produced.
3. As an operator, I want a Reports tab listing every live **Report**
   with its rule, **Target**, and tracker reading, so that I can review
   what has matched without reading raw JSON.
4. As an operator, I want each report row expandable to show the full
   report body pretty-printed - **Reason**, **Penalty**, **Sample**,
   expiry - so that I can judge a report without leaving the page.
5. As an operator, I want to filter the report list with a regular
   expression, so that I can find reports matching a pattern across
   rule, target, and body.
6. As an operator, I want the filter to behave as a search - unanchored
   and case-insensitive - so that I can type a fragment and find it.
7. As an operator, I want an invalid pattern reported inline while the
   list stays unfiltered, so that a typo never blanks my view.
8. As an operator, I want a delete button on each report row, so that I
   can remove a stale or wrong report as part of review.
9. As an operator, I want the delete confirmation to state that the
   enforcement derived from the report will be withdrawn at the
   enforcing consumer's next sync - naming no specific consumer - so
   that I understand deletion touches enforcement, not just storage.
10. As an operator, I want deletion to act on the report's identity -
    rule and target - so that exactly the reviewed report is removed.
11. As an operator, I want to refresh the panel manually, so that I
    control when data reloads while I work through a review.
12. As an operator, I want the panel to arrive inside the daemon binary
    with no external assets and no build step, so that deployment stays
    a single artifact.
13. As an operator, I want the panel on the daemon's existing
    unauthenticated listener, so that no new wiring, TLS, or
    authentication setup is required.
14. As an operator, I want machine API paths under `/api/`, so that
    human-facing pages at the root can never shadow or be shadowed by
    API routes.
15. As a report-consumers' operator, I want the report document served
    from `/api/reports` with its document contract unchanged, so that my
    enforcement pipeline keeps working after I update its URL once.
16. As a report-consumers' operator, I want the shipped documentation
    and service-unit examples to show the new URLs, so that my
    deployment follows tested, current instructions.
17. As a Prometheus operator, I want `/metrics` and `/ping` unmoved at
    the root, so that scrape configs and health probes need no churn.
18. As a rules author, I want a `path regex "..."` matcher, so that I
    can express path patterns like extension families that keyword
    clauses cannot.
19. As a rules author, I want an `agent regex "..."` matcher, so that I
    can group user-agent families with one pattern.
20. As a rules author, I want a bad pattern rejected when the rules are
    parsed, so that a rule can never be silently dead at runtime.
21. As a rules author, I want regex matching to be unanchored like
    keyword matching, so that the DSL's match semantics stay
    consistent across matchers.
22. As a rules author, I want regex matching case-sensitive by default
    with `(?i)` for opt-in insensitivity, so that regex behaves like the
    keyword matchers next to it.
23. As a rules author, I want the regex matcher on the free-form string
    fields - path and agent - so that patterns land where values
    actually vary.
24. As an operator, I want tracker cache Eviction logged at debug level,
    so that steady-state cache pressure stops flooding production logs.
25. As an operator, I want Eviction events still logged at debug level,
    so that I can diagnose cache sizing by raising the log level.
26. As a developer, I want the nginx log parser decoding JSON with
    bytedance/sonic, so that ingest keeps up with high-rate access logs.
27. As a developer, I want the sonic swap confined to the nginx parse
    path and behavior-preserving, so that nothing else in the codebase
    changes its JSON behavior.
28. As a maintainer, I want the API relocation and the sonic adoption
    recorded as ADRs, so that a future reader understands why paths
    moved and why a third-party JSON decoder is in the tree.
29. As a maintainer, I want the glossary to gain the panel term and to
    pin report-deletion and regex-matcher semantics, so that the
    vocabulary stays sharp as the surface grows.
30. As an operator, I want the panel to show only live reports, so that
    what I review is exactly what a consumer would fetch.

## Implementation Decisions

- **API relocation.** The report API and the Statistic move to
  `/api/reports` (GET, DELETE) and `/api/stats`. The Metric endpoint and
  the liveness probe stay at the root as infrastructure endpoints. No
  compatibility aliases for the old paths. The report document contract,
  the Statistic's free shape, and the DELETE semantics (rule + target
  query parameters) are unchanged - only paths move. The in-repo
  consumer's URL documentation, examples, and pinned doc tests update in
  the same change.
- **Web panel.** One page served at `GET /`, two tabs (Statistic view,
  report review), assets embedded in the daemon binary via Go's embed
  mechanism - hand-written HTML, JS, and CSS, no external libraries, no
  build step. Data comes from the same API surface consumers use
  (`/api/stats`, `/api/reports`); the panel adds no data endpoints.
  Manual refresh only.
- **Report review table.** One row per report: rule, target, tracker
  reading, updated time. A row expands to the pretty-printed report
  body. A per-row delete button issues the DELETE against the report's
  (rule, target) identity and, on completion, refetches the list.
- **Delete confirmation copy.** Two-step (click, then confirm). The
  confirmation states that the enforcement derived from the report will
  be withdrawn at the enforcing consumer's next sync. The copy names no
  specific consumer - the enforcing consumer may be any ban-rule
  consumer, not necessarily the in-repo one.
- **Panel regex filter.** One input, applied client-side over the
  fetched report list: unanchored, case-insensitive regex matching across
  rule, target, and stringified body. An invalid pattern shows inline and
  leaves the list unfiltered. This is a panel convenience, deliberately
  separate from the Filter DSL.
- **DSL regex matchers.** New `regex` operator on `path` and `agent`
  only: `path regex "..."`, `agent regex "..."`, with the DSL's existing
  string-literal escaping. Patterns compile at parse time; an invalid
  pattern is a parse error, never a runtime no-match. Matching is an
  unanchored search, case-sensitive by default, with Go's `(?i)` inline
  flag as the case-insensitive opt-in - consistent with the keyword
  matchers' case behavior. `method` and `src` keep their existing
  matchers.
- **Eviction log level.** The tracker cache's eviction event drops from
  warn to debug. Eviction is expected steady-state (the cache fills and
  evicts by design), and nothing alerts on the warn line. No rate
  limiting or sampling is added.
- **JSON decoding.** bytedance/sonic replaces the standard library
  decoder at the nginx log-parse path only, decoding the same line shape
  into the same parsed event. The swap is behavior-preserving; all other
  JSON in the codebase is untouched. The dependency is vendored like the
  tree's existing third-party dependencies; amd64 is the deployment
  target.
- **Documentation.** All references to the old API paths update in the
  same change: the consumer tool's docs and unit examples, the report
  document contract page, the metrics page, and the README. Doc-pinning
  tests update alongside.
- **Design record.** Two new ADRs: the API relocation with pages at the
  root (trade-off vs. compatibility aliases), and sonic for nginx log
  parsing (trade-off vs. the standard library decoder already in use).
  GLOSSARY gains "Web Panel", a deletion-semantics note on Report
  (deletion withdraws the report's enforcement at the enforcing
  consumer's next sync), and regex-matcher semantics under Filter
  (unanchored search; case-sensitive unless `(?i)`).

## Testing Decisions

A good test asserts external behavior - HTTP responses, parse results,
filter decisions, observable log output - never internal structure.
Tests are added at existing seams; no new test seams are introduced.

- **HTTP surface** (the existing handler tests over `httptest`): the
  relocated API paths serve the unchanged report document and
  Statistic; the old paths are gone (404); the root serves the embedded
  panel page and its assets; the Metric and liveness endpoints are
  unmoved; DELETE removes a report by (rule, target) and the next list
  reflects it. Prior art: the existing HTTP API tests.
- **Filter DSL** (the existing parser tests, parse string in, filter
  decision out over synthetic events): `path regex` and `agent regex`
  parse and match unanchored and case-sensitive; `(?i)` flips case
  behavior; an invalid pattern fails parsing with a clear error. Prior
  art: the existing filter parse and match tests.
- **nginx parse** (the existing parser tests and shared bench corpus):
  the sonic swap must pass the existing tests **unchanged** - the tests
  are the proof of behavior preservation. Benchmarks over the corpus
  demonstrate the throughput win that justifies the dependency. Prior
  art: the existing nginx parser tests and bench corpus.
- **Daemon e2e** (the existing log-capture harness): eviction events are
  captured at debug level and absent from warn-level capture. Prior art:
  the existing e2e log assertions on eviction output.
- **Panel JS** (regex filter, confirm dialog, expandable rows): no test
  harness - the panel is deliberately buildless vanilla JS. Covered
  manually per the stories, plus serving tests proving the embedded
  assets load.

## Out of Scope

- Bulk delete, delete-all, or delete-expired affordances (expired
  reports never reach the wire - the repository never returns them).
- Auto-refresh or polling of the panel.
- Authentication, TLS, or a separate listener for the panel.
- Server-side report filtering or search endpoints.
- Regex matchers on `method` or `src`.
- Any change to the report document contract, the Statistic's shape, or
  the Metric series contract.
- Moving `/metrics` or `/ping`.
- A JavaScript test harness or any front-end build tooling.
- Rate-limiting or sampling eviction log lines.
- Behavior changes to enforcement consumers.

## Further Notes

- Deleting a Report withdraws the ban rules a consumer derived from it
  at that consumer's next sync; a consumer that fails to fetch keeps its
  last installed rules, so deletion never causes an enforcement gap
  beyond that sync. The panel copy says "the enforcing consumer"
  generically - deployments may run a different consumer than the
  in-repo one.
- The report repository already bounds reads by expiry, so the panel
  shows only live reports with no extra filtering.
- This spec bundles four independently shippable workstreams (panel +
  API move, eviction log level, DSL regex, sonic). They share one spec
  because they were designed together; commits may land separately.
- The Filter DSL regex matchers operate on Events during rule
  evaluation; the panel's regex filter operates on stored Reports. The
  two are deliberately independent and must not be conflated in docs.
