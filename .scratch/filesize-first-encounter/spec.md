# Spec: First-encounter Filesize enrichment

Status: ready-for-agent

## Problem Statement

The Filesize estimate stays unknown for a path's first few responses:
the enrichment's warm-up gate withholds the derived size until enough
observations have accumulated. Now that observations are the exact
sizes the server reports (Content-Length, Content-Range totals - ADR
0019), the gate no longer protects against noisy transferred-bytes
readings; it only delays knowledge. During that delay the replay
tracker is blind (unknown sizes are skipped entirely), so a path's very
first downloads - exactly the resumable or repeated downloads replay
protection exists for - go unaccounted. The operator sees replay
protection that is deaf until a path has been fetched several times,
and there is no way to make the first response count.

## Solution

The Filesize estimate is known from its path's first observation. When
an enrichment run observes a response size under the Sampling rule and
the path has no record yet, the run creates the record seeded with that
observation and enriches the Event's size in the same run - no warm-up,
no waiting. Later observations refine the estimate by EWMA as before,
and every run on an observed path carries the current estimate. Runs
without a successful observation observe nothing, create nothing, and
carry the estimate only when one exists. An unknown estimate before the
first observation is unchanged: empty content is still indistinguishable
from not-yet-known, and replay stays blind rather than aggressive.

## User Stories

1. As an operator, I want a path's Filesize to be known from its first
   observed response, so that replay protection accounts for a
   download starting with its very first request.
2. As an operator, I want the first observation to enrich the event
   that carried it, so that a resumable download's first segment
   already counts as its true fraction of the file.
3. As an operator, I want the estimate never to flicker from known
   back to unknown while observations continue, so that reports and
   replay volumes never oscillate without cause.
4. As an operator, I want the estimate unknown only until a path's
   first successful observation, so that "unknown" always means
   "this path has never been observed" and never "not enough times
   yet".
5. As an operator, I want replay's blind skipping of unknown sizes
   preserved exactly, so that missing information still never becomes
   a false positive.
6. As an operator, I want later observations to refine the estimate by
   EWMA as before, so that a path's first observation does not freeze
   its size forever.
7. As an operator, I want an observation to seed the estimate exactly
   (no blending at the first sample), so that a 206's Content-Range
   total - the file's exact size - is never smoothed away.
8. As an operator, I want runs without a successful observation to
   leave no state behind, so that scanner noise (404 sweeps, probes)
   cannot fill the file-size cache with empty records and evict real
   estimates.
9. As an operator, I want a run without an observation to still carry
   the current estimate, so that rules after an enrich action see the
   size for every event on an observed path, not only observed ones.
10. As an operator, I want an unobserved path's ineligible runs to
    leave the size unknown, so that a stale or guessed value never
    leaks into a report or a replay increment.
11. As an operator, I want enrichment counters to show first-contact
    learning honestly: a run that observed and enriched counts as
    both, so I can see when paths are first learned from the counters.
12. As an operator, I want the Filesize estimate and its parameters to
    stay unconfigurable, so that no rules file can poison or widen
    what the Sampling rule admits.
13. As an operator, I want the daemon to keep upgrading safely against
    old log-format emitters, so that the estimate stays unknown until
    the emitter sends size fields - now needing just one observation
    after the upgrade instead of several.
14. As a rules author, I want an enrich action's rule to feed
    subsequent rules with a known size from the first matching event,
    so that the documented enrich-then-consume composition works on a
    path's very first request.
15. As a rules author, I want the Sampling rule and the enrich action's
    demand-driven placement to behave exactly as documented (ADR 0028),
    so that nothing about where observations happen changes with this
    feature.
16. As a downstream tooling maintainer, I want reports to carry
    accurate samples and tracker readings from a path's first observed
    response, so that ban rules derived from early reports are not
    based on blind trackers.
17. As a developer, I want "a record exists" to mean "at least one
    observation happened", so that the record's presence itself answers
    whether a path has been observed.
18. As a developer, I want the derived-field reset at evaluation entry
    kept, so that no stale size survives between events even with the
    gate gone.
19. As a developer, I want the glossary and ADR 0019 to read true
    against the merged code, so that no documentation describes a
    warm-up that no longer exists.

## Implementation Decisions

- The Filesize enrichment's warm-up gate is removed entirely: the
  minimum-samples parameter disappears from the enricher's
  construction and its validation, and from the daemon's fixed
  file-size enrichment parameters. The EWMA window (alpha = 1/5) stays
  fixed and unconfigurable by design (ADR 0019).
- The file-size cache port gains a read-only lookup for a path's
  record, alongside its existing atomic update. A record exists
  exactly for paths with at least one observation; the atomic update
  remains the only writer.
- Enrichment run semantics, in full:
  - Successful observation (Sampling rule) on a path with no record:
    the run creates the record seeded exactly with the observation and
    enriches the Event's size with that value in the same run.
  - Successful observation on an observed path: the run blends the
    observation into the EWMA and enriches with the estimate.
  - No successful observation: the run reads nothing of the record's
    write path and never writes; it enriches with the current estimate
    when one exists, else leaves the size at its unknown default.
- The run-outcome contract of the Enrichment counter follows its
  glossary definition unchanged: first successful observation ->
  observed and enriched; later observation -> observed and enriched;
  no observation on an observed path -> enriched only; any run on an
  unobserved path -> neither.
- The single derived-field reset at evaluation entry (ADR 0019) and
  the enricher's overwrite of a stale derived value are unchanged.
- Replay's blind skipping of unknown sizes is unchanged; it simply
  rarely applies now, since one observation suffices.
- Glossary: the Filesize entry is reworded to "a path's estimate
  exists from its first observation and each later observation refines
  it. Unknown until that first observation" (the empty-content
  indistinguishability sentence stays). The word "encounter" does not
  enter the glossary; "first observation" (Sampling rule entry)
  carries the meaning.
- ADR 0019 is amended (its existing amendment note extended) and its
  warm-up-gate consequences reworded; no new ADR, since this adjusts a
  parameter of an existing decision rather than opening a new
  trade-off.

## Testing Decisions

- Good tests state external behavior at existing seams only: what an
  Event's derived size and an enrichment run's outcomes are after
  given traffic, never how the enricher stores or blends internally.
- Two existing seams carry all tests; no new seams:
  - The Filesize enrichment seam (the enricher over its cache port,
    with a stub cache as the test double) - prior art: the enrichment
    package's existing test file and its enrich-once helpers.
  - The assembled-daemon end-to-end seam (rules YAML and raw nginx
    lines in; reports, stats counters and metrics out) - prior art:
    the enrich-action and enrichment-counters e2e test files.
- Modules tested: the Filesize enrichment (seeding, EWMA refinement,
  run-outcome table, stale-value overwrite, per-path state, no cache
  write and no record created without an observation) and the
  assembled daemon's demand-driven enrich path (first-encounter
  enrichment feeding subsequent rules, enrichment-counter
  observed/enriched counts from the first observation, per-rule
  counter exactness under the worker pool).
- The deleted warm-up-gate tests are replaced at the same seams by
  first-encounter cases; the ineligible-events suite keeps its full
  eligibility table and gains the "no record created" assertion.

## Out of Scope

- Changing the Sampling rule or the EWMA window, or making either
  configurable.
- Changing replay's unknown-size skipping semantics, volume methods,
  or transferred-bytes increments.
- Observing error-path totals (416) or active probing (both settled in
  ADR 0019).
- The enrich action's demand-driven placement and per-rule counting
  points (ADR 0028).
- File-size cache entry-cap or eviction policy.

## Further Notes

- The warm-up gate predates ADR 0019: it averaged out the noise of
  transferred-bytes observations. With observations being served
  sizes, the gate only delays knowledge - and a first-encounter-only
  bypass was rejected during design because the estimate would flicker
  known -> unknown -> known across encounters 1, 2-4, 5+.
- Accepted trade-off: a path first observed under gzip seeds from its
  compressed Content-Length and may undershoot until identity
  responses blend in; the increment cap at 1 absorbs the residual skew
  (ADR 0019 consequences).
- Design was stress-tested in a grilling round; the seams check for
  this spec confirmed the two existing test seams above.
