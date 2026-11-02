# The Web Panel replicates pprof's web UI chrome

The **Web Panel**'s chrome - the slim header bar, the VIEW menu with its
dropdown, the `Search regexp` search box, the typography, colors,
uppercase menu items, dropdown and table styling - is a faithful
replication of google/pprof's web UI (its embedded html assets and
shared stylesheet), hand-adapted to the panel's two-view shape. pprof is
light-theme-only and so is the panel. The replication is of the design
language and chrome structure only: upstream's files are templates over
server-rendered data, while the panel's assets are static shells over
client-side fetches from the machine API. pprof's Sample/Refine/Config/
Download menus and its Help link have no domain meaning here and are not
replicated.

The page split lands with it: the panel is two pages instead of one
tabbed page - **Stats** at `/stats` and the report review (Reports) at
`/reports` - and `GET /` redirects to the Stats page. The pages occupy
the web root that ADR 0022 reserved for pages; the machine contract
stays under `/api/`, so no page shadows an API route and each view has
its own bookmarks-able address.

Every derived asset (each page, the stylesheet, the script) opens with a
short notice: derived from google/pprof, Copyright Google Inc., Apache
License 2.0. The obligation is permanent: Apache-2.0 requires the
notice to travel with every copy of the derived assets, so the per-file
notice is the whole licensing deliverable here and must be maintained
for as long as the replication stands - a future cleanup pass must not
strip these comments as noise. (The upstream assets carry no per-file
headers of their own; the notice records provenance where the design is
copied.)

Considered: keeping the bespoke panel skin (rejected: pprof-literate
operators are the panel's exact audience - a leaky-bucket profile
viewer's users already know pprof's chrome - and an instantly
recognisable ops-tool design buys muscle memory a bespoke skin has to
earn); vendoring pprof's assets verbatim (rejected: they are server-side
templates over profile data the panel does not have, and would drag in
views - flame graphs, graphs, source listings - that are out of scope;
hand-adapting keeps the replication to what the two-view shape demands);
one tabbed page instead of two (rejected: a view's address is what makes
it linkable and bookmarks-able - the very split ADR 0022's page
reservation anticipated when it blessed "a future page at `/reports`").

Consequences: operators get the familiarity for the trade of carrying
the permanent notice obligation above; the panel's design is now owned
as a replication, so pprof design changes are not followed - the
replication is of the design at the time of copying; view pages can be
linked and bookmarked independently (the web root redirects to Stats);
and the panel's vocabulary - **Stats** and Reports - is the vocabulary
of record (the snapshot term in the glossary is renamed to **Stats**).
Recorded because the exact-pprof look reads like an accident to a
future reader - it is not, and neither are the per-file notices.
