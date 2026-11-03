# 03: Rule counters on the operator surfaces

**What to build:** A rules author sets `counter: true` on a rule and
the operator sees its Rule counter move on the surfaces they already
read: the `/api/stats` snapshot gains a `rules` block (`id`, `total`,
`matched`, rules-file order, counter-enabled rules only) and the Web
Panel's Stats page gains a rule counters section after the tracker
cache group - a rule/total/matched table narrowed by the shared search
box, with an explicit "no rule counters configured" empty state. The
counts are honest: `total` is every event the rule is evaluated
against (filter rejects and replay warm-up skips included, one visit
per dispatch of a shared chain), `matched` is exactly the rule's
Matches (one per matching Event however many ladder rungs fired),
cumulative since daemon start, reset on restart, exact under the worker
pool. `counter` defaults to false - upgrading adds no noise until
asked for.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A rule-level `counter: true/false` key (default false) is
  accepted by the rules format; strict decode rejects malformed values.
- [ ] `total` increments once per rule visit: filter rejects and
  replay warm-up skips included; a chain dispatched more than once for
  one event counts one visit per dispatch (test pins it).
- [ ] `matched` increments once per Match; a ladder counts one per
  matching Event regardless of rungs fired.
- [ ] Counters are cumulative since daemon start, in-memory only,
  keyed by rule ID in rules-file order, enabled rules only, and exact
  under the worker pool.
- [ ] `/api/stats` carries the `rules` block in rules-file order with
  `{id, total, matched}` per counter-enabled rule.
- [ ] The Stats page shows the rule counters table, narrows it by the
  search box, and shows the explicit empty state when nothing is
  enabled.
- [ ] Tests pin the counting semantics at the daemon end-to-end seam,
  the key handling at the rules-loader seam, and the served surfaces
  at the panel's served-surface seam.

## Comments
