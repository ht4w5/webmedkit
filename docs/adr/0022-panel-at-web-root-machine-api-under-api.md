# The panel is at the web root, the machine API under /api/

The **machine API** - the report document (`GET`/`DELETE`) and the
**Statistic** - lives under the `/api/` prefix, while human-facing pages
occupy the web root and the **Metric** endpoint and liveness probe stay
at the root as infrastructure endpoints. Page routes and API routes can
never shadow each other: a future page at `/reports` is a page, not a
contract, because the contract lives at `/api/reports`. The move is a
move: the report document contract, the Statistic's free shape, and the
DELETE semantics (rule + target query parameters) are unchanged, so a
consumer's migration is one URL.

Considered: compatibility aliases keeping the old `/reports` and
`/stats` root paths serving alongside `/api/` (rejected: the aliases
would keep exactly the bare root paths a future page must not collide
with - the collision this decision exists to end - while doubling the
documented surface and the pinned tests for one contract; a stale
consumer would keep working *silently*, carrying an obsolete URL for as
long as the alias lives, where a loud 404 forces the one-line fix
against unchanged documents); putting the panel somewhere else and
leaving the API at the root (rejected: the web root is where operators
look, and any root page and bare API paths remain one typo apart);
URL-versioning the API (`/api/v1/...`) (rejected: the report document
versions itself with its `version` field and the Statistic is explicitly
unversioned - the URL carries the audience, not the schema).

Consequences: report consumers update one URL - the in-repo consumer
takes it from `--url` and only its docs and unit examples changed - and
fetched documents are byte-identical to what the old paths served; old
root paths 404, so an unmigrated consumer fails visibly at its next
sync instead of drifting; `/metrics` and `/ping` never move, so scrape
configs and health probes need no churn; the root is reserved for pages
(the **Web Panel**) plus those two infrastructure endpoints. Recorded
because the no-aliases stance is a deliberate trade - short-lived
migration pain over permanent dual surface - a future reader will be
tempted to "helpfully" add the old paths back.
