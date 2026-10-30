# Spec: Filesize from response sizes - Content-Length and Content-Range observations

Status: ready-for-agent

## Problem Statement

Replay protection fires on ordinary use. A client downloads a file
normally - maybe seeking within it, maybe revalidating its cache - and
accumulates download-equivalents far faster than it actually
transferred, until the tracker overflows and a report recommends
throttling or banning a legitimate user. The tolerated volume shrinks
over time even when user behavior does not change: the more the site is
browsed (cached revalidations, probes, partial transfers), the smaller
the Filesize estimate becomes, and the smaller the tolerance derived
from it gets. There is no way to see or influence what the estimate is
based on, and no way to keep irrelevant traffic out of the enrichment
path.

## Solution

The Filesize estimate learns from the sizes the server actually reports
for the file - `Content-Length` on GET/HEAD 200 responses and the
`Content-Range` total on GET 206 responses - instead of the bytes each
request happened to transfer. Conditional revalidations (`304`),
`HEAD` without a usable length, error bodies and aborted transfers no
longer move the estimate; a range transfer contributes the file's exact
total and counts as its true fraction of a download toward replay. Both
response headers are carried in the bandage nginx log format.

The enrichment path is gated by a Filter written in the same expression
language as rule filters (default `any`): events it does not match get
default derived values and are otherwise untouched. What feeds the
estimate is fixed in code - the Sampling rule - so no configuration can
poison it. The daemon learns only from observed traffic; it never
requests anything from the protected server itself.

## User Stories

1. As an operator, I want the Filesize estimate to reflect the sizes my
   server reports (Content-Length on GET/HEAD 200, Content-Range totals
   on GET 206), so that replay protection stops firing on ordinary
   downloads.
2. As an operator, I want `304 Not Modified` responses to leave the
   estimate alone, so that heavily cached browsing does not shrink my
   paths' tolerated volumes over time.
3. As an operator, I want `HEAD` requests and health checks to leave the
   estimate alone, so that monitoring traffic cannot starve replay
   tolerance.
4. As an operator, I want `206` range responses to contribute the file's
   full total from `Content-Range`, so that seeking and resumable
   downloads are sized exactly.
5. As an operator, I want a range transfer to count as its true fraction
   of a download, so that resumable downloads neither escape replay
   accounting nor trigger it several times over.
6. As an operator, I want error bodies (404, 403, redirects) to never
   feed the estimate, so that one missing file cannot poison a shared
   path's size.
7. As an operator, I want sizes defined as the server serves them, so
   that the estimate and the transferred bytes replay divides against
   are the same quantity per representation.
8. As an operator, I want bandage to learn purely from observed traffic,
   so that the daemon generates no requests of its own against the
   protected server.
9. As an operator, I want the daemon to run unchanged against old
   flux-format log lines, so that upgrading in either order is safe -
   the estimate simply stays unknown until the log format is upgraded.
10. As an operator, I want an unknown estimate to keep replay blind
    rather than aggressive, so that missing information never becomes a
    false positive.
11. As an operator, I want a malformed `clen` field counted and dropped
    like any other bad log line, so that log corruption is visible and
    never silently corrupts estimates.
12. As an operator, I want unusual `Content-Range` values to degrade to
    "no observation" rather than dropping the event, so that proxy
    quirks do not remove real traffic from analysis.
13. As a rules author, I want a Filter over the enrichment path in the
    same expression language as rule filters, so that I can scope
    enrichment with vocabulary I already know.
14. As a rules author, I want the enrichment Filter to default to `any`
    like rule filters, so that the feature is useful with no
    configuration.
15. As a rules author, I want events outside my Filter to get default
    derived fields and nothing else, so that scoped-out traffic can
    never leak stale or half-computed derived state.
16. As a rules author, I want what feeds the estimate to be fixed in
    code rather than configurable, so that no filter I write can
    accidentally poison the estimate.
17. As a rules author, I want matched-but-ineligible events (chunked
    responses without a length) to carry the path's current estimate, so
    that chunked downloads stay inside replay accounting.
18. As a maintainer, I want the estimate's observation source to be
    response sizes rather than transferred bytes, so that the quantity
    the replay math divides by matches what "one download" means.
19. As a maintainer, I want unmatched events to reset all derived fields
    in one place, so that future enrichments inherit the gate and can
    never leak stale values.
20. As a maintainer, I want the EWMA mechanics untouched (exact seeding,
    integer math, warm-up gate), so that behaviour changes stay
    attributable to the new observation source alone.
21. As a report consumer, I want a report Sample's size to come from
    response sizes, so that ban-rule justifications show the file's real
    size.
22. As an operator, I want the rollout ordering stated plainly (log
    format before daemon), so that I never run the upgrade in the unsafe
    order.
23. As a maintainer, I want the vocabulary (Filesize, Filter, Sampling
    rule, observations) and the rejected alternatives recorded in the
    glossary and an ADR, so that future readers do not re-litigate
    transferred-vs-response sizes.

## Implementation Decisions

1. **Log contract: two response-size fields.** The bandage nginx log
   format gains `clen` (`$sent_http_content_length`) and `crange`
   (`$sent_http_content_range`), both quoted strings, `""` when the
   header was not sent. The Event gains `ContentLength int64` and
   `ContentRangeTotal int64`, where `0` means absent-or-unusable.
   Parsing is asymmetric on purpose: `clen` is a mapped integer field -
   negative or non-numeric values are invalid fields and drop the line,
   counted like other drops; `crange` is parsed leniently -
   `bytes <a>-<b>/<total>` and `bytes */<total>` yield the total, `*`
   totals and anything unparsable yield `0` and never drop the line.
   The parser extracts the total; header syntax never reaches the
   domain. `Sent` keeps its current meaning and uses.

2. **The Filter gates the enrichment path wholly.** The rules file's
   top-level `enrichment` block carries `enrichment.filter` (optional,
   default `any`, compiled by the same expression parser and matcher
   seam as rule filters).
   Unmatched: every derived field is reset to its default in one place -
   the single reset reserved for future enrichments - with no
   observation and no enrichment-state touch. Matched: the event is
   enriched. The whole enrichment block is optional; unknown
   keys and invalid values abort loading naming the key.

3. **The Sampling rule is a hard-coded invariant, not configuration.**
   Only matched events can observe, and among those only: GET or HEAD
   with status 200 and `ContentLength > 0` (observation =
   `ContentLength`), or GET with status 206 and `ContentRangeTotal > 0`
   (observation = `ContentRangeTotal`). The rule is stricter than any
   Filter - a widened filter can never admit `304`, `416`, error or
   headerless traffic into the estimate. Matched but ineligible events
   are enriched with the current estimate and add no observation.

4. **EWMA mechanics unchanged.** Seed with the first observation
   exactly; integer EWMA `prev + (observed-prev)/period`; period 5 and
   the warm-up gate of 5 observations stay hardcoded and not
   configurable. The file-size record type is unchanged. The derived
   size is the estimate from the warm-up gate on, else the unknown
   default - and stays the default for unmatched events regardless.

5. **`416` is not an observation source.** Its `Content-Range: bytes
   */<total>` is authoritative but error-path traffic - strictly
   200/206 per the Sampling rule. Recorded as a future refinement in
   the ADR.

6. **No probing, no fallback.** Active HEAD probing of the protected
   server was considered and rejected (probes re-enter the daemon's own
   ingest as feedback noise, one probe per scanned path amplifies under
   enumeration, one configured origin cannot disambiguate virtual
   hosts). Falling back to transferred bytes when the new fields are
   absent would resurrect the bug and is rejected outright. Both
   rejections are recorded in ADR 0019.

7. **Replay tracker math untouched.** Increments keep dividing
   transferred bytes by the (now accurate) estimated size, capped at
   one; tolerated volumes keep deriving from the estimate via the
   configured volume method. The over-aggression is fixed at the
   estimate, not in the tracker.

8. **Modules touched.** The Event type (two new original fields); the
   nginx log-format definition and parser (two fields, strict/lenient
   split); the enrichment module (gate, reset, Sampling rule,
   observation source); the rules loader and pipeline wiring
   (the enrichment block, filter compilation); documentation (glossary,
   ADR, ingest/rules docs). The evaluation module, tracker math and
   report persistence are unchanged.

## Testing Decisions

Tests assert external behavior only: raw log lines in, derived fields
and evaluation outcomes out - never EWMA internals beyond the observable
size sequence, never parser internals beyond parse/drop outcomes.

Seams (all existing; this feature introduces no new seam - removing
active probing removed the only one that had been proposed):

1. **Primary acceptance seam: the app end-to-end pipeline** (raw nginx
   JSON line through parse, enrich and evaluate), the highest seam in
   the codebase. It carries every acceptance behavior of this feature:
   both new fields flowing into sizes; the strict-`clen` drop (counted)
   and lenient-`crange` degradation (event kept); the gate's
   wholly-in/wholly-out semantics including zero enrichment-state
   touches for unmatched events; Sampling-rule admit/reject tables,
   including a widened filter failing to poison the estimate; the
   warm-up window; and range transfers counting as fractions in replay
   outcomes. Prior art: the app's existing end-to-end pipeline suites.
2. **Existing enricher seam** (stub file-size cache): kept only for the
   EWMA arithmetic and warm-up rows its unit tests already pin,
   re-pinned to response-size observations. Prior art: the enricher's
   integer-math table tests.
3. **Existing parser seam** (one line in, one event or error out): the
   strict-vs-lenient field tables for `clen` and `crange`. Prior art:
   the parser's current field-validation table tests.
4. **Existing rules-loading seam** (strict decode): the rules file's
   `enrichment` block compiles or aborts loading naming the key;
   unknown enrichment keys abort. Prior art: the rules loader's
   block-validation tests.

Benchmarks run at the pipeline bench seam with the new observation path;
they must show no regression.

## Out of Scope

- Active probing of the protected web server (any form).
- Any `Sent`-byte fallback for the estimate, flag or otherwise.
- Filter-language predicates over `clen`/`crange` (the Sampling rule
  owns them; the DSL vocabulary is unchanged).
- `416` observations (future refinement).
- Changes to replay tracker math, volume methods, the tracker cache,
  report persistence, or the EWMA arithmetic.
- Making the period or the warm-up gate configurable.
- Enrichments other than Filesize (the derived-field reset reserves the
  place for them).

## Further Notes

- **Amended after implementation:** the enrichment settings live in the
  rules file's `enrichment` block, not the application config - the
  rules author owns them, and `enrichment:` in `bandage.yml` is
  rejected as an unknown key.
- **Already recorded during design:** `GLOSSARY.md` redefines **Filesize**
  (learned from observed response sizes) and **Filter** (decides whether
  a rule or the enrichment path considers an event), and adds
  **Sampling rule** (fixed, non-configurable; not a Filter). Prose calls
  the estimate's inputs *observations*, reserving **Sample** for the
  report concept. `docs/adr/0019-filesize-from-response-sizes.md`
  records the decision, the rejected alternatives (probing, `416`,
  fallback) and the consequences below.
- **GZIP caveat:** sizes are as served - compressed paths blend
  compressed and identity observations in one estimate. Per-
  representation increments stay wire-consistent and the increment cap
  at one absorbs the residual skew; operators who want exactness disable
  compression for such content.
- **Rollout ordering:** deploy the updated nginx log format before
  upgrading bandage. In the other order everything runs safely, but the
  estimate stays unknown (replay blind) until the emitter is upgraded
  and paths are re-observed.
- **Future refinements:** `416` totals as observations; filter
  predicates for response sizes; a max-based estimator alongside the
  EWMA if mixed representations ever matter.
