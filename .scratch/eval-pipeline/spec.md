# Evaluation pipeline: configurable reason prefix, non-terminal reports, rule counters - spec

Status: ready-for-agent

## Problem Statement

Three frictions in the evaluation pipeline, each felt by a different
person at a different moment.

A rules author who wants their reports to name their deployment rather
than the binary cannot: the **Reason** prefix is hardwired to the
program name (ADR 0017), so every report a bandage instance produces
is namespaced `bandage.` no matter which product, rebrand, or customer
instance it protects. Two instances feeding one downstream consumer
collide in one namespace they never chose.

A rules author whose chain carries a report action keeps losing
evidence: a report is **terminal**, so the first rule that flags a
**Target** ends chain traversal for that **Event** and every rule after
it never sees it. The reasons accruing across rules are exactly what
justifies a ban downstream, yet whether they accrue depends on rule
order and on which rule happened to report first - a control-flow
consequence nobody wrote down, riding on a write action.

And an operator wondering "is this rule drowning in traffic or asleep?"
has no answer: the **Stats** snapshot counts lines and tracker-cache
traffic, but nothing about the rules doing the work. Debug logs are
the only window, and they do not survive a quiet hour.

## Solution

The rules author gets one top-level `reason_prefix` key: every report
the file's rules produce carries `<prefix>.<code>`, and an absent key
means the program-name prefix exactly as today - nothing to migrate.

Report actions stop ending chain traversal: a Trigger writes and the
event continues through the chain's remaining rules. Ending traversal
becomes an explicit act - a `stop` action, or `goto`'s existing
"target chain, then this chain ends" - exactly where the author wrote
it.

And each rule may carry `counter: true`, giving it a **Rule counter**:
the **Events** it is evaluated against and the subset that **matched**
it, cumulative since daemon start. The counts show up everywhere the
operator already looks - the `/api/stats` snapshot, the **Web Panel**'s
Stats page, and the Prometheus **Metrics** contract - so "how loaded is
this rule?" is a glance or a query away, and "this rule's matched rate
just spiked" is an alert.

## User Stories

1. As a rules author, I want a top-level `reason_prefix` key in my
   rules file, so that my reports namespace under my deployment's name
   rather than the binary's.
2. As a rules author, I want the configured prefix to apply to every
   report my file's rules create, so that one file has exactly one
   namespace and my reasons never fragment.
3. As a rules author, I want an absent `reason_prefix` to mean the
   program-name prefix, so that my existing rules files keep producing
   byte-identical reasons with no edit.
4. As a rules author, I want the prefix validated as a slug at load
   time, so that a typo fails `bandage -t` instead of silently
   renaming my namespace at runtime.
5. As a rules author, I want the load error to name the key, so that I
   fix the right line on the first try.
6. As a rules author, I want the `--test` plan to print the effective
   reason prefix, so that I can see which namespace a deployment will
   use before I ship it.
7. As a report consumer, I want the reason to keep its `<prefix>.
   <code>` shape and my authored code as its second label, so that the
   configurable prefix changes attribution and nothing else.
8. As a report consumer, I want the report document contract untouched
   by this change, so that my validation logic keeps working unchanged.
9. As a rules author, I want a report action to let the event continue
   through my chain's remaining rules, so that one flag does not blind
   every rule written after it.
10. As a rules author, I want `stop` to remain the explicit way to end
    chain traversal, so that ending traversal is my decision, written
    where I wrote the rule.
11. As a rules author, I want `goto` to keep its "target chain, then
    this chain ends" meaning and `jump` to stay non-terminal, so that
    my dispatch structure means exactly what it meant before.
12. As a rules author, I want an escalation ladder unchanged by the
    terminality fix, so that every crossed rung still fires lowest
    first and no action cancels a fired rung.
13. As a rules author, I want to enable a Rule counter per rule with
    `counter: true`, so that I watch the rules I care about and leave
    the rest quiet.
14. As a rules author, I want `counter` to default to false, so that
    upgrading adds no noise to my stats until I ask for it.
15. As an operator, I want a Rule counter to count `total`, the events
    the rule is evaluated against, so that I see the load each rule
    actually bears.
16. As an operator, I want `total` to include events the filter
    rejected and replay warm-up skips, so that the number is the real traffic
    hitting the rule, not just what got through.
17. As an operator, I want `matched` to count exactly the rule's
    Matches, so that I can compare offered load against detections.
18. As an operator, I want a ladder rule to count one `matched` per
    matching Event however many rungs fired, so that the count matches
    "events this rule flagged", not rung traffic.
19. As an operator, I want counters to be cumulative since daemon
    start and reset on restart like every other Stats number, so that I
    read them the same way I read the pipeline counters.
20. As an operator, I want the counters to be exact under the worker
    pool, so that a busy daemon's numbers are trustworthy.
21. As an operator, I want a `rules` block in `/api/stats` listing
    each counter-enabled rule's `id`, `total` and `matched` in my rules
    file's order, so that the snapshot reads top to bottom like my
    config.
22. As an operator, I want a rule counters section on the Web Panel's
    Stats page, so that the numbers are on screen beside the pipeline
    and tracker cache blocks.
23. As an operator, I want an explicit "no rule counters configured"
    empty state in that section, so that silence is never mistaken for
    a broken panel.
24. As an operator, I want the panel's search box to narrow the rule
    counter rows like every other row, so that I find one rule in a
    long file.
25. As a scraper, I want `bandage_rule_events_total{rule}` and
    `bandage_rule_events_matched_total{rule}` counter families in the
    Metrics contract, so that I alert on a rule's matched rate spiking
    instead of polling a human surface.
26. As a scraper, I want the rule families in the exposition after the
    tracker-cache families with series in rule-ID order, so that the
    contract's deterministic rendering holds and diffs stay readable.
27. As a scraper, I want enabled rules' series present from daemon
    start with zeros, so that `rate()` and `increase()` work before the
    first match.
28. As a scraper, I want the metrics contract document to state the
    families, their HELP text and the amended cardinality guarantee, so
    that the machine contract and the exposition cannot drift.
29. As an operator, I want the periodic stats log line unchanged, so
    that my log volume does not grow with my rules file.
30. As a rules author, I want the shipped example rules audited for
    report terminality, so that the behavior change is visible where I
    copy my config from.

## Implementation Decisions

- The rules format (version 0, unchanged) gains two keys: a top-level
  `reason_prefix` beside the version/enrichment/chain material, and a
  rule-level `counter` boolean. Strict decode is unchanged; unknown
  keys still fail load; errors follow the loader's established
  key-naming style.
- `reason_prefix` is slug-shaped - the authored code's pattern - and
  validated at load. The loader carries the optional value through; the
  effective prefix (configured value, else the program name) is chosen
  at daemon assembly, where the program name is known. Report creation
  namespaces with the effective prefix exactly where the program-name
  prefix composes today (ADR 0016: namespaced at creation only); the
  build identity and version stamps keep the program name regardless.
- The program name keeps its slug-shape guard at the prefix-composition
  seam as the fallback's pin; the configured value's guard is load
  validation.
- Terminality is a static property of a rung's action list and shrinks
  to `goto` and `stop`: a report action writes and lets the event
  continue through the chain's remaining rules; `jump` is unchanged;
  the fired-rungs decision (decided once pre-action, lowest first) is
  unchanged. This amends the ladder documentation's terminality
  statement and is recorded as a decision superseding report
  terminality.
- A Rule counter is the glossary term: per-rule event accounting, two
  quantities, `total` (events evaluated against the rule) and `matched`
  (the subset that matched it - the glossary's Match: filter accepts
  and the tracker overflows; tracker-less rules match on filter
  alone). Not a Tracker: it counts events per rule, never accumulates a
  quantity per Target.
- Counting points live in the evaluator's rule loop - the evaluator
  owns what counts; the daemon owns storage and exposure. `total`
  increments once per rule visit: filter rejects and replay warm-up
  skips included, and a chain dispatched more than once for one event
  counts one visit per dispatch. `matched` increments once per Match,
  a ladder's fired rungs counting once.
- The counter store is a small in-memory registry keyed by rule ID -
  loader-enforced unique across the file - in rules-file order, holding
  atomic counters only for rules with `counter: true`. It is
  cumulative-since-start with no persistence; restart resets it, like
  every other Stats number. The registry is an internal seam between
  evaluator and daemon, not a public contract.
- `/api/stats` (still explicitly without a format contract) gains a
  `rules` block: an array of objects carrying `id`, `total` and
  `matched`, rules-file order, counter-enabled rules only. The snapshot
  keeps its field-for-field mirror relationship with the metric
  families.
- The Metrics contract gains two counter families, rendered after the
  tracker-cache families: `bandage_rule_events_total{rule}` (events
  evaluated against the rule since daemon start) and
  `bandage_rule_events_matched_total{rule}` (events that matched the
  rule since daemon start). Series render in rule-ID order; enabled
  rules emit from daemon start with zeros. The exposition stays
  hand-rendered per the metrics-are-a-hand-rendered-contract decision;
  adding series is the contract's stated evolution policy, so no
  version or ADR change accompanies the addition.
- The metrics contract document's cardinality promise is amended from
  fixed-for-every-family to fixed-per-configuration: the rule families'
  series count is the number of counters enabled in the rules file and
  is never a function of traffic. The document's labels paragraph
  widens to three label-carrying families, and its pinned example is
  regenerated with the new families.
- The Web Panel's Stats page gains a rule counters section after the
  tracker cache group: a three-column table (rule, total, matched) in
  the established table styling, narrowed by the shared search box, with
  an explicit empty state when no rule enables a counter. The panel's
  tolerance for unknown snapshot keys rendering as generic rows is
  unchanged.
- The periodic stats log record is unchanged by decision: rule counters
  are a query-time surface.
- The rules format's `--test` plan prints the effective reason prefix;
  the shipped example rules are audited for report terminality and
  annotated where a chain's later rules now run.

## Testing Decisions

A good test here observes external behavior at the highest seam that
already exists: events in, served documents and reports out - never
registry internals, never evaluator call counts. Where a lower seam is
the established home for a concern (load errors, contract pins), tests
stay there.

- **The daemon end-to-end harness** carries most of the spec: rules
  YAML and raw log lines in, `/api/reports`, `/api/stats`, `/metrics`
  and captured logs out, with an injected clock. It pins: the
  configured prefix on served reports and the absent-key fallback to
  the program name; report-then-continue beside `stop`/`goto`
  terminality and unchanged ladder traversal; every counter semantic
  (total-per-visit including filter rejects and replay warm-up skips,
  one match per Match, ladder-once, one visit per dispatch of a shared
  chain) through the `rules` block; the block's order and shape; and
  the zero-from-start rule series. Prior art: the existing `TestE2E*`
  suites for triggers, escalation and suppression.
- **The rules loader** pins load-time behavior the e2e seam reaches
  awkwardly: slug rejection of `reason_prefix`, malformed `counter`
  rejection, key-naming error style, absent-key defaults. Prior art:
  the rules and shipped-format tests.
- **The plan printer and contract pins** pin the rest: the `--test`
  plan's effective-prefix line; the metrics contract example
  byte-for-byte against the regenerated document; the panel's table
  slots and empty-state copy as served-surface assertions. Prior art:
  the shipped-format tests, the metrics doc pin test, the panel
  served-surface tests.
- No new test seams are introduced; the counter registry is exercised
  only through the seams above.

## Out of Scope

- Per-rule or per-report-action reason prefixes: one file, one
  namespace stands (the fragmentation rejection survives; only the
  file-wide source moved).
- Per-rung or per-threshold counters: counters are per rule.
- Persisting counters, counter history, or windows/rates computed in
  the daemon - consumers derive rates from the cumulative counters.
- Any metric family rename, removal, or new label dimension (no chain
  label): additions only.
- Changes to Trigger semantics (live-report suppression, in-place
  override), tracker accumulation, or the report document contract
  (stays at version 0).
- The periodic stats log record's shape.
- New action vocabulary beyond the terminality shift (no continue
  modifier, no per-action flags).

## Further Notes

- This is a behavior change for rules files that relied on a report
  action ending the chain: they must add `stop` to keep their old
  traversal. The shipped example configs are audited and the change is
  called out in the release notes.
- The domain glossary gains the **Rule counter** term (already
  recorded), explicitly distinct from the Tracker's avoided "counter"
  word.
- Two decisions are recorded as ADRs alongside this spec: the reason
  prefix's move to the rules file (superseding the program-name
  decision) and the terminal set shrinking to goto and stop (amending
  the escalation-ladder premise). The metrics addition rides the
  contract's stated add-series policy and needs no ADR.
- The tickets under this feature's `issues/` directory decompose this
  spec into implementable pieces: reason prefix, non-terminal report,
  rule counters, and the counter stats surface (the last blocked by
  the counters themselves).
