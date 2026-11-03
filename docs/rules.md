# Writing rules

Rules live in the rules file (`configs/rules.yml` is a complete
example): `version: 0`, a list of chains, each chain an ordered list
of rules. Per event, evaluation enters exactly one chain - the
**entry chain** named `main` - and every other chain is a subroutine
run only when a `goto` or `jump` action names it
([Entry chains and dispatch](#entry-chains-and-dispatch) below). A
**Rule** is an id, a filter, at most one tracker, and an ordered list
of actions (report, log, goto, jump, stop) - or an [escalation
ladder](#escalation-ladders) of action lists instead of one. The
filter expression language and the full DSL are specified below in
[Filter expressions and enrichment.filter](#filter-expressions-and-enrichmentfilter);
the DSL's design is recorded in
[ADR 0001](adr/0001-dsl-flux-chains-webjail-filters.md).

Two glossary entries govern what happens at runtime
([GLOSSARY.md](../GLOSSARY.md),
[ADR 0008](adr/0008-trigger-semantics-one-live-report.md) as amended by
[ADR 0013](adr/0013-escalation-ladders-in-place-escalation.md)):

- **Match** - the rule's filter accepts the Event and its tracker (if
  any) overflows; the rule's actions then run - or, on a ladder, the
  actions of every rung the value exceeds, lowest first. A rule
  without a tracker matches on the filter alone.
- **Trigger** - a Match that ran the rule's report action. The report
  action is the only write path to reports.

Trigger semantics hold for every rule (ADR 0008, amended by ADR 0013):

- One **Report** per (rule, Target) per report lifetime. While the
  report for (rule, Target) is alive, a Trigger at or below the volume
  recorded in its `tracker` reading - the threshold of the rung that
  last wrote it - writes nothing: no downgrade and no rewrite. A rule
  without a ladder never overrides its live report: one write per
  report lifetime. A Trigger from a strictly higher rung overrides the
  report **in place**:
  reason, penalty, rate, Sample, tracker reading, update time and
  expiry all move to the overriding rung's values, and its `ttl` runs
  from that write. Only `created_at` stays frozen at first flagged. The
  rule itself is untouched: it keeps matching and its other actions
  keep running per event.
- A Trigger never empties the rule's tracker for the Target (or the
  (Target, path) records of a replay tracker). The record keeps
  accumulating across writes - landed or suppressed - so a Tracker used
  as a threshold gate for downstream rules never flickers shut on a
  write. Only the tracker cache's TTL and eviction restart a record.
- A failed report write is not a Trigger outcome: the tracker stays
  intact and the next matching event retries the report.
- Deleting a report (`DELETE /api/reports`) or letting its `ttl` expire
  re-arms writing: the next matching event writes a fresh report at the
  accumulation's current stance - a dead report's stance is not
  remembered, and one event's writes climb the crossed rungs, each
  overriding the last. Its Sample and tracker reading come from that
  event and the accumulation as it stands.

Validate config and rules before deploying: `bandage -t -c bandage.yml
-r rules.yml` prints the validated plan and exits without ingesting.

## Filter expressions and `enrichment.filter`

Rule filters and the enrichment gate share one expression language over
Event fields - predicates `any`, `none`, `src 10.0.0.0/8`, `method GET`,
`status 200`, `status class 2`, `path "/exact"`,
`path prefix|suffix|keyword|regex "text"` (`agent` takes the exact,
`keyword`, and `regex` forms, plus `agent prefix|suffix "text"` -
matching the user agent's literal start or end, exactly like
`path prefix|suffix`),
`sent >= 1m` / `sent <= 1m`, `size >= 100m` / `size <= 1m`, and
`size unknown`
(size units `k`, `m`, `g` are binary - `1k` is 1024 bytes - and are
allowed only after `sent` and `size`) -
combined with `and`, `or`, `not`, where `not` binds tighter than `and`,
which binds tighter than `or`.

Text values - the `path`/`agent` exact form and the
`prefix`/`suffix`/`keyword`/`regex` modifiers - come in two spellings,
interchangeable everywhere. Double-quoted strings process four
escapes: `\"` is a quote, `\\` a backslash, `\n` a newline, `\t` a
tab; any other `\X` aborts loading with `unknown escape sequence \X`.
Single-quoted strings have exactly one escape: `''` is a single quote.
Every other character - backslashes included - is literal, and the
string ends at the first unpaired quote, so `'foo\'` is `foo\` and
`''''` is one literal quote (`''''''` is two). Both spellings may
span newlines, and empty text is rejected wherever a text value is
required (the `path`/`agent` exact form and the
`prefix`/`suffix`/`keyword`/`regex` modifiers) under either spelling.
Single quotes are the recommended spelling for regex patterns
(`path regex '\.iso$'`): the pattern reads exactly as the regex engine
sees it - in the double-quoted spelling a regex `\.` must instead be
doubled to `"\\."`.

`sent` decides over the bytes the event actually transferred; `size`
decides over the served content's Filesize estimate
([GLOSSARY.md](../GLOSSARY.md)) - never conflated. A `size` comparison
is inclusive at the boundary and considers only events whose Filesize
is known: an event whose size is unknown is never claimed by `size >=`
or `size <=`. `size unknown` selects exactly those unknown-size events,
the complement of every comparison; its own complement is spelled
`not size unknown` - there is deliberately no `size known` synonym and
no equality operator on `size`. The degenerate limits are well-defined
because a learned size is never 0: `size >= 0` matches every known
size and `size <= 0` matches nothing. Size 0 means unknown
([ADR 0024](adr/0024-size-0-means-unknown.md)) - empty content is
indistinguishable from not-yet-known
([GLOSSARY.md](../GLOSSARY.md)).

`path regex '…'` and `agent regex '…'` compile their pattern when the
rules load - an invalid pattern aborts loading, never a silent
no-match at runtime. Matching is an unanchored search over the Event's
path or user agent, case-sensitive unless the pattern carries Go's
`(?i)` inline flag (e.g. `agent regex '(?i)curl'`). These matchers
decide over Events during rule evaluation; the web panel's report
filter is a separate client-side search over stored Reports, and the
two are deliberately independent.

`enrichment.filter` - the rules file's top-level `enrichment` block,
between `version` and `chains` - gates the enrichment path (the
Filesize estimate) with the same language, defaulting to `any` like
rules. Enrichment settings live in the rules file, not `bandage.yml`;
an invalid expression or an unknown key in the block aborts loading,
naming the key. The gate is wholly in or wholly out: an unmatched event
flows through evaluation with default derived fields and touches no
enrichment state - no observation, no cache update. A widened filter
cannot poison the estimate: what feeds it is the **Sampling rule**
([GLOSSARY.md](../GLOSSARY.md)), fixed in code and stricter than any
Filter ([ADR 0019](adr/0019-filesize-from-response-sizes.md)).

Beware `size` in `enrichment.filter`: the gate runs after the derived
fields are reset and before the Filesize estimate is looked up, so every
event reaches it with an unknown size - `size unknown` there is a no-op
gate like `any`, while a `size` comparison never matches and silently
disables enrichment. Deliberately accepted, not rejected at load.

```yaml
version: 0
enrichment:
  filter: path prefix "/downloads/"
chains:
```

## `reason_prefix`

The top-level `reason_prefix` key - beside `version`, `enrichment` and
`chains` - names the namespace every report of the file carries: a
reason is `<prefix>.<code>`, the code being the bare slug the report
action declares
([ADR 0016](adr/0016-report-reasons-are-namespaced.md),
[ADR 0026](adr/0026-reason-prefix-configured-in-rules-file.md)). One
file, one namespace: the key is file-wide, never per rule or per
action.

The prefix is slug-shaped like the authored code -
`^[a-z0-9][a-z0-9_-]*$` - and validated at load: a typo fails
`bandage -t` naming the key, never renaming the namespace silently at
runtime. An absent key and a written key are all-or-nothing: absent
means the default below, while a written key must carry the slug - a
written null or non-string value fails load like any other typo. When
the key is absent the prefix is the program name exactly
as before
([ADR 0017](adr/0017-reason-prefix-comes-from-the-progname.md) kept as
the default), so existing rules files keep producing byte-identical
reasons with no edit. Only reasons move: the build identity and the
version stamps keep the program name regardless. The `--test` plan
prints the effective prefix as `reason prefix: <prefix>` above the
chains and namespaces every `report reason=` line with it.

```yaml
version: 0
reason_prefix: acme-prod
chains:
```

## Entry chains and dispatch

Every event is evaluated by entering exactly one chain: the **entry
chain**, the chain named `main` ([GLOSSARY.md](../GLOSSARY.md),
[ADR 0015](adr/0015-main-chain-entry.md)). Every other chain is a
subroutine, entered only when a `goto` or `jump` action names it -
the firewall shape rule authors already know: one entry chain with a
single front door, helper chains dispatched on demand.

- `jump` runs the target chain, then resumes this chain at its next
  rule; `goto` runs the target chain, then ends this chain (its later
  rules are skipped for the event - the fired action list still runs
  in full). Dispatch is the only way to enter any chain but `main`.
- A chain nothing dispatches to never runs. Drafts and shared library
  chains are inert, like iptables custom chains - and chain
  declaration order never matters: only the dispatch does.
- `stop` in the entry chain gates everything below it: later rules and
  later dispatch never see the event.
- A `report` action is a write, not a control-flow act
  ([ADR 0027](adr/0027-only-goto-and-stop-are-terminal.md)): it writes
  the report and the event continues through the chain's remaining
  rules. Ending traversal is explicit - `stop`, or `goto`'s "target
  chain, then this chain ends" - exactly where the rules author wrote
  it; reasons accrue across rules and one flagged report no longer
  blinds everything written after it.

Load refuses a rules file where:

- no chain is named `main` - the entry chain is required, so a
  misconfigured document fails at startup instead of silently
  processing nothing;
- a `goto` or `jump` action targets `main` - the entry chain has
  in-degree 0, and dispatching back to it would be dead dispatch (the
  event is already being evaluated there).

The dispatcher pattern - the shipped example's front door. `main`
carries one filter-less `jump` rule per topical chain, in file order;
a `jump` is non-terminal, so the dispatcher sequences exactly like the
topical chains' own file order, and the escalation ladder's handoff
target (`inspect`) is deliberately *not* dispatched from `main` (see
the pitfall below):

```yaml
  - name: main
    rules:
      - id: dispatch-download-abuse
        actions: [{jump: download-abuse}]
      - id: dispatch-api
        actions: [{jump: api}]
      - id: dispatch-iso-egress
        actions: [{jump: iso-egress}]
```

**The double-dispatch pitfall.** Dispatch keeps plain subroutine
semantics and is deliberately not deduplicated (ADR 0015): a chain
reached twice for one event runs **twice** for that event, feeding its
trackers twice. A diamond - `main` jumping to `shared` both directly
and through a middle chain - overflows `shared`'s trackers at half the
intended volume. Give each event one dispatch path to a chain: the
shipped example leaves `inspect` to the ladder's `jump` rung alone,
because dispatching it from `main` as well would double-feed its
replay tracker on every handoff event.

## `penalty` and `rate`

Every report action declares a **Penalty** - the enforcement stance the
report recommends for the Target - with a `penalty` key beside `reason`
and `ttl` ([GLOSSARY.md](../GLOSSARY.md),
[ADR 0010](adr/0010-reports-carry-an-advisory-penalty.md)). The
vocabulary is closed to five values; the limit penalties carry a `rate`:

| `penalty` | recommends | `rate` |
| --- | --- | --- |
| `drop` | discard the target's traffic silently | none |
| `reject` | refuse the target's traffic with an error signal | none |
| `limit-pkt-rate` | throttle to `rate` packets per second | required |
| `limit-bandwidth` | throttle to `rate` response-payload bytes per second | required |
| `limit-req-rate` | throttle to `rate` requests per second | required |

`rate` is a plain count (`200`) or a decimal human size (`1MB` =
1000000), optionally suffixed `/s` - the same quantity language as the
tracker's `leak` and `volume`. Once parsed, the rate must be a whole
number ≥ 1: a fractional count (`1.5`) is refused, while a fractional
human size (`1.5MB` = 1500000) is accepted - the serialized rate is an
integer. The penalty name states the unit: the rate of
`limit-bandwidth` counts response-payload bytes, the bytes the report's
Sample records as `sent_bytes`. Every limit is aggregate over the
report's Target - the whole prefix, not each host inside it.

`bandage -t` prints the penalty and rate of every report action, so
every recommendation can be audited before deploying:

```
    action report reason=bandage.iso-egress-overflow ttl=168h0m0s penalty=limit-bandwidth rate=1000000
```

Load refuses a rules file where:

- `penalty` is missing or is not one of the five values;
- `rate` accompanies `drop` or `reject`;
- a `limit-*` penalty has a missing, zero, fractional or unparseable
  `rate`.

The penalty is advice and nothing more: bandage validates it for
well-formedness only - never against the rule's tracker or reason
(ADR 0010). Rules sharing one reason may recommend different penalties,
a tripwire may recommend a limit and a tracked rule `drop`, and whether
- or how restrictively - a consumer applies the penalty is the
consumer's decision. bandage produces reports and never ban rules
(ADR 0004).

Worked examples - the three idiomatic shapes:

The tracker-less tripwire flags immediately and recommends matching
immediate action: `drop`, no rate (full pattern below):

```yaml
      - id: ua-trip
        filter: 'agent keyword "curl"'
        actions:
          - report:
              reason: ua-trip
              ttl: 24h
              penalty: drop
```

A bandwidth-tracked rule recommends `limit-bandwidth`, its rate in
response-payload bytes per second:

```yaml
      - id: sustained-iso-egress
        tracker: {track: bandwidth, leak: 1MB, volume: 100GB}
        actions:
          - report:
              reason: iso-egress-overflow
              ttl: 7d
              penalty: limit-bandwidth
              rate: 1MB
```

A rate-tracked rule recommends `limit-req-rate`, its rate in requests
per second:

```yaml
      - id: bursty-api
        tracker: {track: rate, leak: 10, volume: 200}
        actions:
          - report:
              reason: api-rate-overflow
              ttl: 12h
              penalty: limit-req-rate
              rate: 200
```

These pairings are idiomatic, not enforced: `limit-pkt-rate` (packets
per second) and `reject` round out the vocabulary for rules whose
traffic calls for them.

## Escalation ladders

A rule trades its action list and its `tracker.volume` for an
**Escalation** ladder ([GLOSSARY.md](../GLOSSARY.md),
[ADR 0013](adr/0013-escalation-ladders-in-place-escalation.md)): an
`escalations` list of two or more rungs, each an `at` threshold in the
tracker's quantity and its own ordered action list. One tracker
accumulates the target's traffic and the response climbs as the value
does. The shipped rules config carries the worked ladder below
(`iso-egress-ladder` in [`configs/rules.yml`](../configs/rules.yml)): a
handoff rung, a throttle recommendation, a ban:

```yaml
      - id: iso-egress-ladder
        tracker: {track: bandwidth, leak: 1MB}
        escalations:
          - at: 50MB
            actions: [{jump: inspect}]
          - at: 5GB
            actions:
              - report:
                  reason: iso-egress-throttle
                  ttl: 24h
                  penalty: limit-bandwidth
                  rate: 1MB
          - at: 40GB
            actions:
              - report:
                  reason: iso-egress-ban
                  ttl: 7d
                  penalty: drop
```

Each rung's `at` is a quantity in the tracker's kind and nothing else:
plain request counts for `rate`, human byte sizes (`50MB`, `5GB`) for
`bandwidth`, download equivalents for `replay` - the same quantity
language as `volume`, so `50MB` on a rate rule is a load error and not
a runtime surprise.

How it behaves:

- A ladder rule matches when the filter accepts and the accumulated
  value exceeds a rung. The fired rungs are decided **once** from
  the post-accumulation value, before any action runs: every rung the
  value exceeds fires, lowest first. Thresholds ascend strictly at
  load, so the fired rungs are always a prefix of the ladder - a
  transfer that flew past rungs cannot skip the lower responses, and
  actions within the event can never cancel a rung that already fired.
- Rungs fire on every matching event while the value exceeds
  them, and stop as `leak` decays it below. A handoff rung therefore
  feeds its downstream chain continuously, not in flickers.
- Terminality is declared per rung's action list, and applies only to
  rungs that fired: the chain ends after the rule only when a fired rung
  carried `goto` or `stop`
  ([ADR 0027](adr/0027-only-goto-and-stop-are-terminal.md), amending
  ADR 0013's terminality set). A fired `report` is a write, not a
  control-flow act: it writes and the event continues through the
  chain's remaining rules. A fired `jump` runs its target chain and
  resumes; a fired `goto` runs its target chain and ends this chain
  after the remaining fired rungs have run. An unfired rung above the
  value ends nothing.
- The idiomatic handoff rung is **`jump`, not `goto`**: a `goto` rung
  ends the chain on every event above its threshold, starving the rules
  after the ladder, while `jump` keeps them running beside the handoff.
- A report rung writes exactly like a single-volume rule's report
  action (the Trigger semantics above): one Report per (rule, Target)
  whose stance escalates **in place**. A Trigger from a strictly higher
  rung overrides the live Report - reason, penalty, rate, Sample, the
  tracker reading, update time and expiry move to the overriding rung's
  values - while a rung at or below its recorded threshold is silent;
  only `created_at` stays frozen at first flagged, so the Sample always
  backs the report's current stance. Rung reasons may be distinct or
  shared; each report action carries its own `ttl` (the ban's lifetime
  runs from the ban rung) and `penalty`.
- `log` actions on any rung log the same rule, Target, Event and
  tracker reading detail as today; the reading's `volume` is the firing
  rung's threshold.
- The tracker cache-entry TTL derives from the ladder's top rung, so
  tracker state outlives the whole climb (the zero-leak default and the
  one-second minimum are unchanged); a replay ladder's TTL re-derives
  per event from the same top rung.

Load refuses, each error naming the rule: a ladder beside `actions` or
beside `tracker.volume` - a scalar volume is the single-volume rule a
ladder is never declared on, and a volume-method block (`constant`,
`clamped-linear`, `inverse-square`, `step`) stays exclusive to the
single-volume form; a ladder without a tracker; fewer than two rungs;
`at` thresholds that are not strictly ascending; and thresholds that do
not parse in the tracker kind's quantity - bytes for bandwidth
(`50MB`), plain counts for rate, download equivalents for replay
(already normalized by Filesize: `2` means two full downloads of that
path whether it is a 1MB or a 10GB file). Each rung's actions are
validated by the same action parser as rule actions, and rung
`goto`/`jump` targets join the same acyclic chain-graph validation.

Replay measures traffic in download equivalents: each event counts its
transferred fraction of the path's **Filesize** - the estimated total
size of the path's content, learned from response sizes
(`Content-Length` on GET/HEAD 200, `Content-Range` totals on GET 206),
never from the bytes a request happened to transfer
([GLOSSARY.md](../GLOSSARY.md),
[ADR 0019](adr/0019-filesize-from-response-sizes.md)). While the
estimate is unknown - warm-up, or a log emitter without the
response-size fields - replay is blind and the event adds nothing to
the tracker.

The shipped ladder's `bandage -t` output shows every rung with its
threshold and its actions; the tracker line carries no volume (the
rungs carry every threshold) and its `ttl` from the top rung:

```
  rule iso-egress-ladder
    filter: any
    tracker: bandwidth leak=1000000 ttl=11h6m40s
    rung at=50000000 jump target=inspect
    rung at=5000000000 report reason=bandage.iso-egress-throttle ttl=24h0m0s penalty=limit-bandwidth rate=1000000
    rung at=40000000000 report reason=bandage.iso-egress-ban ttl=168h0m0s penalty=drop
```

## Rule counters

A rule may carry the rule-level `counter: true` key (default `false`),
enabling its **Rule counter** ([GLOSSARY.md](../GLOSSARY.md)): the
events the rule is evaluated against and the subset that matched it,
cumulative since daemon start and reset on restart like every other
Stats number. As with `reason_prefix`, the key is all-or-nothing: an
absent key means the default, while a written `counter` must carry
`true` or `false` - a written null fails load like any other typo.
Two quantities, counted exactly under the worker pool:

- `total` counts every event the rule is evaluated against - events
  its filter rejected and replay warm-up skips included. A chain
  dispatched more than once for one event counts one visit per
  dispatch.
- `matched` counts exactly the rule's Matches - one per matching
  Event, a ladder counting once however many rungs fired.

The counts surface on the operator's Stats surfaces: the `/api/stats`
snapshot's `rules` block lists every counter-enabled rule as `id`,
`total` and `matched` in rules-file order, and the Web Panel's Stats
page shows the same as its rule counters table. Rules without
`counter` count nothing and stay out of both.

```yaml
version: 0
chains:
  - name: main
    rules:
      - id: iso-download-flood
        counter: true
        tracker: {track: rate, leak: 10, volume: 100}
        actions: [{report: {reason: iso-download-flood, ttl: 24h, penalty: limit-req-rate, rate: 200}}]
```

## Pattern: the tracker-less tripwire

The tripwire flags every request carrying a known-bad signal (classically
a user agent) immediately - no accumulation, no volume: an agent keyword
filter plus a report action, and **no tracker**. Its writes are bounded
by report suppression alone (see the Trigger semantics above).

```yaml
version: 0
chains:
  - name: main
    rules:
      - id: ua-trip
        filter: 'agent keyword "curl"'
        actions:
          - report:
              reason: ua-trip
              ttl: 24h
              penalty: drop
```

How it behaves:

- Every matching event is a Match and therefore a Trigger - there is no
  tracker to overflow.
- Its `penalty: drop` recommends discarding the offending source's
  traffic - the stance a flag this serious warrants. `drop` carries no
  `rate` (see [penalty and rate](#penalty-and-rate) above).
- The first Trigger writes one Report per (rule, Target). Further
  Triggers while that report is alive write nothing (the Sample and the
  update time freeze at the first write): the flag is written once per
  offending source, not once per request.
- The **Target** is the event's source aggregated at the default
  prefix - per address for IPv4, per /64 for IPv6. A tracker-less rule
  has no `target_prefix` to configure; each offending source gets one
  report of its own.
- Re-arm: `DELETE /api/reports?rule=ua-trip&target=203.0.113.7` or expiry
  at `ttl` re-arms writing - the next matching event writes a fresh
  report immediately. A still-offending source keeps matching (and its
  other actions keep running) while suppressed, so nothing is lost by
  the silence.
- A restart does not re-arm a live report: the live-report set is
  derived from the report table at startup (the table is the
  correctness source, ADR 0008).
- `reason` is the machine-readable slug downstream tooling turns into a
  ban rule (ADR 0004): a lowercase slug `^[a-z0-9][a-z0-9_-]*$`,
  shared freely between rules; the rule id is internal provenance.
  bandage produces reports and never ban rules. Codes are authored bare
  and carried namespaced: the report carries the code namespaced as
  `<prefix>.<code>` (ADR 0016) under the file's
  [`reason_prefix`](#reason_prefix) - the program name when the key is
  absent
  ([ADR 0026](adr/0026-reason-prefix-configured-in-rules-file.md)) -
  while the rules file declares it bare - the plan prints the
  namespaced reason the report will carry.
- The filter above is a case-sensitive substring match on the Event's
  user agent (`agent "…"` matches the agent exactly). Values match
  case-sensitively without normalization (a `regex` pattern is
  case-sensitive too, unless it carries `(?i)`); `and`, `or` and `not`
  are reserved and must be quoted when used as values.

Rules with a tracker work the same way with one difference in the
Match semantics: a Match additionally requires the tracker to overflow
(on a ladder, exceed a rung - then the fired rungs' actions run,
lowest first). Nothing empties the tracker on a Trigger, so a deleted
or expired report is rewritten on the next matching event, its Sample
and tracker reading reflecting the accumulation as it stands.
