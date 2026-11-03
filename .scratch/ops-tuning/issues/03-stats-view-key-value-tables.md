# 03: Stats view as grouped key/value tables

**What to build:** The Stats page renders the daemon's snapshot - the
same human-facing document the machine API serves, with no format
contract (ADR 0006) - as grouped key/value tables in pprof's table
styling: identity, pipeline, tracker cache. Keys the panel doesn't know
appear as generic extra rows within their group, so counters added to
the snapshot later are never silently hidden by a fixed table. The
header's search box narrows the visible key/value rows the way it
narrows report rows on the other view. The right-aligned legend shows
the build identity and uptime, so the running binary's identity is on
screen at a glance.

**Blocked by:** 01 (Web Panel chrome - per-view pages in pprof's design)

**Status:** resolved

- [x] The snapshot renders as grouped key/value tables (identity,
  pipeline, tracker cache) instead of a JSON blob.
- [x] Unknown keys in the snapshot render as generic rows in their
  group - nothing the daemon reports is dropped by the panel.
- [x] The search box narrows the key/value rows shown.
- [x] The legend shows build identity and uptime.
- [x] Served-surface tests cover the table slots and the legend copy;
  interactive behaviour is verified manually per the spec's user
  stories.

## Comments

Resolved on `integration/ops-tuning`: merged in `a164258` (merge: 03-stats-view-key-value-tables into integration/ops-tuning (Stats view as grouped key/value tables)); code-review fix pass merged in `1d261e2`. All checklist items are pinned by the tests named in the spec's Testing Decisions. Interactive panel behaviour remains on the manual user-story checklist per the panel's testing convention.
