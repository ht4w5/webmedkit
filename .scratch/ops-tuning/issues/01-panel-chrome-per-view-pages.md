# 01: Web Panel chrome - per-view pages in pprof's design

**What to build:** The Web Panel becomes two pages instead of one tabbed
page: the web root redirects to the Stats page, and the Reports page
serves at its own page address under the web root (ADR 0022 reserved
page paths there; the machine API stays under the API prefix and is
untouched). Both pages are static shells fetching the machine API
client-side and share a replicated google/pprof web UI header: brand
link to the Stats page, a VIEW dropdown listing Stats and Reports with
pprof's active-item styling on the current view, the regexp search box
whose placeholder is `Search regexp`, a REFRESH menu item (the only
reload trigger - nothing polls), and a right-aligned legend slot. The
shared stylesheet carries pprof's design language - its typography,
colors, uppercase menu items, dropdown styling, table and link styling,
light theme only. Both views still carry today's content (the snapshot's
JSON rendering, the report review table) so the panel is fully usable
the moment this lands; the view-level payoffs come in the follow-up
tickets. Every hint paragraph is gone - the search placeholder is the
panel's only affordance text - and every served asset opens with a short
notice: derived from google/pprof, Copyright Google Inc., Apache License
2.0. The glossary's snapshot term becomes **Stats** (avoid: statistic,
metrics) and its Web Panel definition matches the new chrome; ADR 0025
records the replication, its trade-off, the permanent license-notice
obligation, and the page split.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The web root redirects to the Stats page; both view pages serve as
  HTML from the daemon's embedded assets; the machine API and
  infrastructure endpoints are unchanged and no page shadows an API
  route.
- [x] Both pages serve the replicated header: VIEW dropdown with Stats
  and Reports (current view styled as active), `Search regexp` search
  box, REFRESH item, right-aligned legend slot.
- [x] The panel looks like pprof's web UI: typography, colors, menu and
  table styling match the replicated design language; light theme only.
- [x] Both views remain functional on their new pages - the snapshot
  renders (JSON rendering acceptable at this stage) and the review table
  lists live reports with its existing affordances.
- [x] No hint copy remains anywhere in the panel.
- [x] Every served asset carries the short google/pprof Apache-2.0
  notice in its header.
- [x] REFRESH refetches both views' data; there is no polling.
- [x] Served-surface tests cover the new routes (including the web-root
  redirect), the header chrome, the absent hints, and the license
  notices; all existing tests pass.
- [x] The glossary reflects the **Stats** term rename and the Web Panel
  definition; ADR 0025 exists.

## Comments

Resolved on `integration/ops-tuning`: merged in `dee6d75` (merge: 01-panel-chrome-per-view-pages into integration/ops-tuning (Web Panel chrome - per-view pages in pprof design)); code-review fix pass merged in `1d261e2`. All checklist items are pinned by the tests named in the spec's Testing Decisions. Interactive panel behaviour remains on the manual user-story checklist per the panel's testing convention.
