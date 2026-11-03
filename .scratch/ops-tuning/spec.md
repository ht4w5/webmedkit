# Web Panel mirrors the pprof web UI, quiet happy-path logs, in-memory report store - spec

Status: ready-for-agent

## Problem Statement

Operators running bandage look at the **Web Panel** all day and get very
little from it. The **Stats** tab is a raw JSON dump in a `<pre>`, so
finding one counter means scanning an unstructured blob; the panel's
bespoke styling looks like nothing else in an operator's toolbelt; and
the tab shell carries three paragraphs of hint text explaining API
endpoints - trivia that repeats itself after the first visit and buries
the two things the panel actually shows. pprof-literate operators - the
exact audience for a leaky-bucket profile viewer - get none of the
familiarity they already have.

The daemon also floods its own log on the happy path: the periodic
statistics record fires on every sweep tick at info level, and the
sweep-deleted record joins it whenever anything expires, so steady-state
operation drowns real events in repeated self-reporting.

And when a developer wants to debug against the report store, every run
touches on-disk state - there is no first-class way to point the daemon
at a throwaway database, so debugging means cleaning up database files
or carefully arranging paths that exist only in one's head.

## Solution

The Web Panel becomes a faithful replica of google/pprof's web UI
chrome: the same header bar, the same uppercase menu items and dropdown,
the same regexp search box, the same typography, table and button
styling. The two views are called **Stats** and **Reports**, selected
from a VIEW dropdown exactly the way pprof selects its views. The Stats
view renders the snapshot as grouped key/value tables instead of a JSON
blob; the Reports view keeps its review table - delete and row detail
stay - in pprof's table skin. All hint copy is gone; the search box's
`Search regexp` placeholder is the only affordance text the panel needs.
Each derived asset carries a short license notice naming google/pprof
and Apache-2.0.

The happy-path log flood ends: the periodic statistics record and the
sweep-deleted record drop to debug level. Failures still shout; routine
self-reporting whispers.

Debugging gets a first-class in-memory mode: a config option selects an
in-memory report store, warned about at startup because reports stop
being durable - deliberately, and only in this mode.

## User Stories

1. As a bandage operator, I want the Web Panel to look and behave like
   the pprof web UI, so that my existing muscle memory works and the
   panel feels like part of my standard ops toolbelt.
2. As a bandage operator, I want a VIEW dropdown in the panel header
   listing Stats and Reports, with the current view marked the way pprof
   marks its active view, so that I always know where I am and can switch
   views the way I switch pprof views.
3. As a bandage operator, I want the two views named "Stats" and
   "Reports", so that the panel's vocabulary matches how I talk about
   what the daemon shows me.
4. As a bandage operator, I want each view served as its own page
   (redirected from the web root, and at a bookmarks-able address), so
   that I can link a colleague straight to the reports review without
   clicking through tabs.
5. As a bandage operator, I want a regexp search box in the header like
   pprof's, so that I can narrow what I'm looking at without leaving the
   page or reading instructions.
6. As a bandage operator, I want the search box to filter the report
   review rows by rule, target, and body with an unanchored,
   case-insensitive regex, so that I can find the one report I care about
   in a long list.
7. As a bandage operator, I want the search box to also narrow the key/
   value rows on the Stats view, so that the search box behaves the same
   whichever view I'm on.
8. As a bandage operator, I want every hint paragraph removed from the
   panel, so that the page shows my data instead of explaining itself to
   me on every visit.
9. As a bandage operator, I want the Stats view to show the daemon's
   snapshot as grouped key/value tables - identity, pipeline, tracker
   cache - in pprof's table styling, so that I can find a counter at a
   glance instead of scanning raw JSON.
10. As a bandage operator, I want counters the panel doesn't know about
    to still appear as extra rows, so that new daemon counters are never
    invisible to me just because the panel predates them.
11. As a bandage operator, I want the Reports view to keep its review
    table - rule, target, tracker reading, updated - in pprof's table
    skin, so that reviewing live reports stays exactly as useful while
    looking native to the pprof design.
12. As a bandage operator, I want to click a report row to see its full
    body - reason, penalty, sample - inline, so that I can judge a report
    without leaving the page or curling the machine API.
13. As a bandage operator, I want a Delete control on each report row,
    so that I can withdraw a report (and the enforcement derived from it)
    from the panel.
14. As a bandage operator, I want a REFRESH control in the header bar,
    so that I can reload both views' data on demand - and only on demand,
    nothing polls behind my back.
15. As a bandage operator, I want a right-aligned legend in the header -
    build identity and uptime on Stats, live report count on Reports - so
    that the most-run-for facts are on screen no matter which view I'm in.
16. As a bandage operator, I want the panel served entirely from the
    daemon binary with no external asset fetches, so that the panel works
    on an air-gapped monitoring host exactly like anywhere else.
17. As a bandage operator, I want the empty report set to render a clear
    empty state rather than a bare table, so that "nothing is live" reads
    as information, not as a broken page.
18. As a bandage operator, I want an invalid search regex to be called
    out inline instead of silently matching nothing, so that I can fix my
    query rather than misread the data.
19. As a log reader, I want the periodic statistics record to log at
    debug level, so that the steady-state log carries events, not the
    daemon narrating its own counters every sweep tick.
20. As a log reader, I want the sweep-deleted record to log at debug
    level, so that routine expiry doesn't punctuate the log on every
    sweep that deletes something.
21. As a log reader, I want sweep failures and the API-started record to
    keep their current levels, so that bad news is still loud and the
    daemon's arrival is still visible.
22. As a bandage developer, I want to run the daemon against an in-memory
    report store via one config option, so that debugging sessions leave
    no database files behind and start from an empty store every time.
23. As a bandage developer, I want the config to reject a memory-mode
    setting combined with a database path, so that I can't misconfigure
    myself into guessing which one won.
24. As a bandage developer, I want the daemon to warn at startup when it
    runs with the in-memory store, so that nobody mistakes a debug run
    for a durable one - reports in this mode do not survive restarts.
25. As a bandage operator, I want each panel asset to carry a short
    license notice naming google/pprof and Apache-2.0, so that the
    provenance of the design is documented where it is copied, and our
    licensing obligations are met wherever the code travels.
26. As a maintainer, I want the glossary's snapshot term renamed to
    "Stats" and the Web Panel definition updated to match the new chrome,
    so that the domain vocabulary keeps matching what operators actually
    see.
27. As a maintainer, I want the pprof replication recorded as an ADR, so
    that a future reader who wonders "why does this look exactly like
    pprof?" finds the trade-off and the license obligation spelled out.

## Implementation Decisions

- The Web Panel's HTML, CSS and JS are re-authored as a faithful
  replication of google/pprof's web UI (its embedded html assets and
  shared stylesheet), adapted only where our two-view shape demands it.
  pprof is light-theme-only and so is this.
- Replicated chrome: a slim fixed-height header bar in pprof's colors,
  pprof's typography and table styling (sticky right-aligned table
  headers, its link and row-highlight colors), uppercase menu items with
  dropdown submenus in pprof's structure: one VIEW menu whose submenu
  lists Stats and Reports, with pprof's active-item styling on the
  current view. pprof's Sample/Refine/Config/Download menus have no
  domain meaning here and are not replicated; neither is a Help link
  (no help page exists).
- The header carries a regexp search box (placeholder `Search regexp`),
  a REFRESH menu item, and a right-aligned legend: build identity and
  uptime on Stats, live report count on Reports.
- The two views become separate static pages instead of one tabbed page:
  the web root redirects to the Stats page's own address, and the
  Reports page sits at a page path under the web root that ADR 0022
  explicitly reserved for pages (the machine contract stays under the
  API prefix and is untouched). Each page is a static shell that fetches
  the machine API client-side, per the Web Panel definition; the shared
  header is duplicated in both shells rather than introducing server-side
  templating. Embedded asset names mirror pprof's (shared css/js, one
  page per view).
- The Stats view renders the snapshot as grouped key/value tables
  (identity / pipeline / tracker cache). Unknown keys render as generic
  extra rows: the snapshot's "fields may be added" freedom (ADR 0006)
  stays visible rather than being silently dropped by a fixed table.
- The Reports view keeps the review table's Delete column and
  click-to-expand detail rows; only the skin mirrors pprof. The search
  box's report filtering keeps its semantics (unanchored,
  case-insensitive regex over rule, target, and body - the same
  behaviour the panel's old report filter had), and an invalid regex is
  shown inline.
- All hint copy is removed. The search box placeholder is the only
  affordance text; there is no explanation of endpoints, reload
  semantics, or filter syntax on the page.
- Every derived asset (each page, the stylesheet, the script) opens with
  a short comment notice: derived from google/pprof, Copyright Google
  Inc., Apache License 2.0. No separate license file - the per-file
  notice is the whole obligation here (Apache-2.0 requires preserving
  notices; the upstream assets carry no per-file headers of their own).
- The Web Panel keeps its current data discipline: fetched client-side
  from the machine API, reloaded only on manual refresh (the REFRESH
  item and page loads), no polling, no data endpoints of its own.
- Log-level change: the periodic statistics record and the sweep-deleted
  record (emitted only when something was deleted) drop to debug level.
  The sweep failure record stays at error and the API-started record
  stays at info - the goal is to silence repeated happy-path
  self-reporting, not to hide state changes or bad news.
- The sqlite report store gains a memory mode selected by a config
  option under its driver settings (the driver-keyed store config in the
  daemon config). The option is mutually exclusive with the store's path
  setting: setting both is a config validation error. In memory mode the
  repository opens an in-memory database - its existing shared-connection
  no-WAL path - so read and write paths share one database. The existing
  "in-memory database named by the path" behaviour keeps working as
  before.
- Startup in memory mode logs a warning that reports are not durable:
  this deliberately bends ADR 0002 (durable reports) for debugging, and
  the record must say so at the moment the operator can still act on it.
- Domain model: the snapshot term in the glossary is renamed to "Stats"
  (its avoid list becomes "statistic, metrics", keeping the distinction
  against Metric); the Web Panel definition loses "tabbed" and gains the
  menu-selected per-view pages, the search bar, and the REFRESH-only
  reload discipline.
- ADR 0025 records: the pprof replication and its trade-off (operator
  familiarity from an instantly recognisable ops-tool design vs a
  bespoke panel skin), the permanent Apache-2.0 provenance-notice
  obligation on the derived assets, and the page-address split.

## Testing Decisions

Good tests here assert external behaviour only: bytes and status codes
on the wire, the results of config parsing, and the log records the
daemon emits. The panel's JavaScript internals are never a test subject;
where behaviour lives in JS alone it is verified manually per the user
stories (this is the established convention for the panel).

- **Served-surface seam (the httpapi package's test server and request
  helpers, as used by the panel and API-path tests) - the highest seam,
  and the only one for the Web Panel work.** Tests assert: the route
  table (web root redirecting to the Stats page, both view pages
  served, the machine API and infrastructure endpoints unchanged, no
  shadowing between page and API routes); the served pages carrying the
  replicated chrome (VIEW dropdown with the Stats and Reports entries,
  search box, REFRESH item, legend) and none of the removed hint copy;
  the Stats page carrying the key/value table slots; the Reports page
  carrying the review table with its delete and detail affordances; and
  each served asset carrying its license notice. Prior art: the existing
  web panel serving tests and the API-path tests in the same package.
- **Config seam (the daemon config parser, as in the config tests).**
  Tests assert: memory mode alone parses; memory mode together with a
  path is a validation error; memory mode under any other driver is
  rejected like the rest of the store config. Prior art: the existing
  config parse-valid / parse-errors tests.
- **Daemon e2e log-capture seam (the daemon end-to-end tests that
  capture the log stream, as in the eviction-debug and level-filtering
  tests).** Tests assert: the periodic statistics record and the
  sweep-deleted record are emitted at debug and absent at info; the
  sweep failure record still logs at error; the API-started record still
  logs at info; and startup in memory mode emits the non-durable warning.
  Prior art: the existing statistics-log-agrees-with-stats and
  eviction-logged-at-debug tests.
- **Report store seam (the sqlite repository's own tests).** The
  in-memory database's split-path behaviour is already covered and the
  repository contract does not change; no new tests are required there
  beyond what the config seam's wiring exercises end to end.

## Out of Scope

- Any change to the machine API's paths, documents, or the report
  document format - the panel consumes them unchanged.
- Dark mode or any theme other than pprof's light theme.
- A Help page, pprof's Sample/Refine/Config/Download menus, dialogs,
  flame graphs, charts, or any pprof view we have no data for.
- Auto-refresh, polling, or streaming; reload stays manual.
- Server-side templating or a build step for the panel assets.
- A CLI flag for the memory store (config-only for now; the flag can
  come later if debugging workflows demand it).
- Any further log-level changes than the two happy-path records named
  here.
- The metrics contract: no series is added, renamed, or removed.

## Further Notes

- The design source is google/pprof's embedded web assets at the time of
  implementation; the replication is of its design language and chrome
  structure, hand-adapted - the upstream files are templates over
  server-rendered data, ours are static shells over client-side fetches.
- Interactive panel behaviour (dropdown open/close, search filtering,
  row expansion, REFRESH) has no automated harness by convention; the
  user stories above are the manual verification checklist.
- The change set is deliberately small and independent in three parts:
  the panel replication, the two log-level demotions, and the memory
  store option. They can land together or separately.
