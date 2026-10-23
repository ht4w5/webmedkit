# Writing rules

Rules live in the rules file (`configs/rules.yml` is a complete
example): `version: 0`, an ordered list of chains, each chain an
ordered list of rules. A **Rule** is an id, a filter, at most one
tracker, and an ordered list of actions (report, log, goto, jump,
stop) — or an [escalation ladder](#escalation-ladders) of action lists
instead of one. The filter expression language and the full DSL are specified
in [PLAN.md](../PLAN.md) §1.

Two glossary entries govern what happens at runtime
([GLOSSARY.md](../GLOSSARY.md),
[ADR 0008](adr/0008-trigger-semantics-one-live-report.md) as amended by
[ADR 0013](adr/0013-escalation-ladders-in-place-escalation.md)):

- **Match** — the rule's filter accepts the Event and its tracker (if
  any) overflows; the rule's actions then run. A rule without a tracker
  matches on the filter alone.
- **Trigger** — a Match that ran the rule's report action. The report
  action is the only write path to reports.

Trigger semantics hold for every rule (ADR 0008, amended by ADR 0013):

- One **Report** per (rule, Target) per report lifetime. While the
  report for (rule, Target) is alive, further Triggers write nothing:
  the report freezes — its Sample and update time stay at the first
  write. The rule itself is untouched: it keeps matching and its other
  actions keep running per event.
- A Trigger never empties the rule's tracker for the Target (or the
  (Target, path) records of a replay tracker). The record keeps
  accumulating across writes — landed or suppressed — so a Tracker used
  as a threshold gate for downstream rules never flickers shut on a
  write. Only the tracker cache's TTL and eviction restart a record.
- A failed report write is not a Trigger outcome: the tracker stays
  intact and the next matching event retries the report.
- Deleting a report (`DELETE /reports`) or letting its `ttl` expire
  re-arms writing: the next matching event writes a fresh report whose
  Sample and tracker reading come from that event and the accumulation
  as it stands.

Validate config and rules before deploying: `bandage -t -c bandage.yml
-r rules.yml` prints the validated plan and exits without ingesting.

## `penalty` and `rate`

Every report action declares a **Penalty** — the enforcement stance the
report recommends for the Target — with a `penalty` key beside `reason`
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
1000000), optionally suffixed `/s` — the same quantity language as the
tracker's `leak` and `volume`. Once parsed, the rate must be a whole
number ≥ 1: a fractional count (`1.5`) is refused, while a fractional
human size (`1.5MB` = 1500000) is accepted — the serialized rate is an
integer. The penalty name states the unit: the rate of
`limit-bandwidth` counts response-payload bytes, the bytes the report's
Sample records as `sent`. Every limit is aggregate over the
report's Target — the whole prefix, not each host inside it.

`bandage -t` prints the penalty and rate of every report action, so
every recommendation can be audited before deploying:

```
    action report reason=iso-egress-overflow ttl=168h0m0s penalty=limit-bandwidth rate=1000000
```

Load refuses a rules file where:

- `penalty` is missing or is not one of the five values;
- `rate` accompanies `drop` or `reject`;
- a `limit-*` penalty has a missing, zero, fractional or unparseable
  `rate`.

The penalty is advice and nothing more: bandage validates it for
well-formedness only — never against the rule's tracker or reason
(ADR 0010). Rules sharing one reason may recommend different penalties,
a tripwire may recommend a limit and a tracked rule `drop`, and whether
— or how restrictively — a consumer applies the penalty is the
consumer's decision. bandage produces reports and never ban rules
(ADR 0004).

Worked examples — the three idiomatic shapes:

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
does:

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

How it behaves:

- A ladder rule matches when the filter accepts and the accumulated
  value stands above a rung. The fired rungs are decided **once** from
  the post-accumulation value, before any action runs: every rung the
  value stands above fires, lowest first. Thresholds ascend strictly at
  load, so the fired rungs are always a prefix of the ladder — a
  transfer that flew past rungs cannot skip the lower responses, and
  actions within the event can never cancel a rung that already fired.
- Rungs are level-triggered: they fire on every matching event while
  the value stays above them, and stop as `leak` decays it below. A
  handoff rung therefore feeds its downstream chain continuously, not
  in flickers.
- Terminality is declared per rung's action list, and applies only to
  rungs that fired: the chain ends after the rule only when a fired rung
  carried `report`, `goto` or `stop`. A fired `jump` runs its target
  chain and resumes; a fired `goto` runs its target chain and ends this
  chain after the remaining fired rungs have run. An unfired rung above
  the value ends nothing.
- The idiomatic handoff rung is **`jump`, not `goto`**: a `goto` rung
  ends the chain on every event above its threshold, starving the rules
  after the ladder, while `jump` keeps them running beside the handoff.
- A report action on a rung writes exactly like a single-volume rule's
  report action (the Trigger semantics above): one Report per
  (rule, Target), silent while that report is alive. Rung reasons may be
  distinct or shared; each report action carries its own `ttl` and
  `penalty`.
- `log` actions on any rung log the same rule, Target, Event and
  tracker reading detail as today; the reading's `volume` is the firing
  rung's threshold.
- The tracker cache-entry TTL derives from the ladder's top rung, so
  tracker state outlives the whole climb (the zero-leak default and the
  one-second minimum are unchanged); a replay ladder's TTL re-derives
  per event from the same top rung.

Load refuses, each error naming the rule: a ladder beside `actions` or
beside `tracker.volume` — a scalar volume is the single-volume rule a
ladder is never declared on, and a volume-method block (`constant`,
`clamped-linear`, `inverse-square`, `step`) stays exclusive to the
single-volume form; a ladder without a tracker; fewer than two rungs;
`at` thresholds that are not strictly ascending; and thresholds that do
not parse in the tracker kind's quantity — bytes for bandwidth
(`50MB`), plain counts for rate, download equivalents for replay
(already normalized by Filesize: `2` means two full downloads of that
path whether it is a 1MB or a 10GB file). Each rung's actions are
validated by the same action parser as rule actions.

`bandage -t` prints every rung with its threshold and its actions; the
tracker line carries no volume (the rungs carry every threshold) and
its `ttl` from the top rung:

```
  rule iso-egress-ladder
    filter: any
    tracker: bandwidth leak=1000000 ttl=11h6m40s
    rung at=50000000 jump target=inspect
    rung at=5000000000 report reason=iso-egress-throttle ttl=24h0m0s penalty=limit-bandwidth rate=1000000
    rung at=40000000000 report reason=iso-egress-ban ttl=168h0m0s penalty=drop
```

## Pattern: the tracker-less tripwire

The tripwire flags every request carrying a known-bad signal (classically
a user agent) immediately — no accumulation, no volume: an agent keyword
filter plus a report action, and **no tracker**. Its writes are bounded
by report suppression alone (see the Trigger semantics above).

```yaml
version: 0
chains:
  - name: trip
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

- Every matching event is a Match and therefore a Trigger — there is no
  tracker to overflow.
- Its `penalty: drop` recommends discarding the offending source's
  traffic — the stance a flag this serious warrants. `drop` carries no
  `rate` (see [penalty and rate](#penalty-and-rate) above).
- The first Trigger writes one Report per (rule, Target). Further
  Triggers while that report is alive write nothing (the Sample and the
  update time freeze at the first write): the flag is written once per
  offending source, not once per request.
- The **Target** is the event's source aggregated at the default
  prefix — per address for IPv4, per /64 for IPv6. A tracker-less rule
  has no `target_prefix` to configure; each offending source gets one
  report of its own.
- Re-arm: `DELETE /reports?rule=ua-trip&target=203.0.113.7` or expiry
  at `ttl` re-arms writing — the next matching event writes a fresh
  report immediately. A still-offending source keeps matching (and its
  other actions keep running) while suppressed, so nothing is lost by
  the silence.
- A restart does not re-arm a live report: the live-report set is
  derived from the report table at startup (the table is the
  correctness source, ADR 0008).
- `reason` is the machine-readable slug downstream tooling turns into a
  ban rule (ADR 0004): a lowercase slug `^[a-z0-9][a-z0-9_-]*$`,
  shared freely between rules; the rule id is internal provenance.
  bandage produces reports and never ban rules.
- The filter above is a case-sensitive substring match on the Event's
  user agent (`agent "…"` matches the agent exactly). Values match
  case-sensitively without normalization; `and`, `or` and `not` are
  reserved and must be quoted when used as values.

Rules with a tracker work the same way with one difference in the
Trigger semantics: a Match additionally requires the tracker to
overflow. Nothing empties the tracker on a Trigger, so a deleted or
expired report is rewritten on the next matching event, its Sample and
tracker reading reflecting the accumulation as it stands.
