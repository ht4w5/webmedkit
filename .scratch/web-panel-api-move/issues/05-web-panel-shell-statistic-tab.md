# 05: Web panel shell with Statistic tab at the web root

**What to build:** `GET /` serves the Web Panel - a single embedded page
on the daemon's existing unauthenticated listener: hand-written HTML, JS,
and CSS compiled into the binary, no external assets, no build step. The
first tab renders the Statistic snapshot fetched from `/api/stats` for
operator eyes. A manual Refresh control refetches. Two tab slots exist
(Statistic, reports), with the reports tab arriving in a later ticket.

**Blocked by:** 04 - Move the machine API under `/api/`

**Status:** resolved

- [x] `GET /` serves the panel page and its embedded assets from the existing listener with no extra configuration.
- [x] The Statistic tab renders the `/api/stats` snapshot.
- [x] Refresh is manual and refetches on demand.
- [x] No external assets are fetched; the daemon binary alone serves the whole panel.
- [x] The glossary gains the Web Panel term.

## Answer

Implemented on `ticket/wam-05-web-panel-shell` (commit `42fc2b0`,
fast-forwarded onto `integration/web-panel-api-move`). `GET /` serves
the Web Panel - one page plus panel.css/panel.js embedded via Go embed
(the tree's first), hand-written, buildless, no external assets - on
the existing unauthenticated listener (exact-root `GET /{$}` pattern).
The Statistic tab renders the `/api/stats` snapshot; a manual Refresh
refetches (no polling); the Reports tab was a marked placeholder until
ticket 06 filled it. Serving tests prove the page and its assets load
and nothing external is fetched; the e2e `GET /` 404 pin flipped to
panel-serving. GLOSSARY gains the Web Panel term. Reviewed in the fix
pass `62e3efa`.
