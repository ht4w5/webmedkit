# Filesize is learned from response sizes, not transferred bytes

Replay increments and volumes divide by the Filesize (GLOSSARY), which
was learned from `$body_bytes_sent` observations on every event - so `304`
and `HEAD` responses (zero sent bytes), gzip, `206` ranges and aborted
transfers all dragged the estimate far below the file, saturating replay
increments and shrinking tolerated volumes into over-aggressive replay
protection. The estimate now observes full-response sizes only - the
**Sampling rule** (GLOSSARY): `Content-Length` on GET/HEAD 200 and the
`Content-Range` total on GET 206, both carried in the bandage nginx log
format as `clen` and `crange`. Parsing is asymmetric on purpose: a
malformed `clen` is an invalid field and drops the line like any bad log
line, while an unusual `crange` degrades to no observation and never
drops the event. Nothing falls back to transferred bytes: an emitter
without the new fields leaves the estimate unknown (the existing
warm-up blindness) until it is upgraded and the path re-observed.

Two gates keep the layers honest. The user-defined **Filter** (the
rules file's `enrichment` block, `enrichment.filter`, default `any`
like rules) decides which events the
enrichment path considers at all; unmatched events are fully out -
default derived fields, no observation, no cache touch - and
matched-but-ineligible events are enriched without observing. The
Sampling rule is fixed, not configurable: a widened Filter can never
poison the estimate.

Considered: active HEAD probing of the protected server for exact sizes
(rejected: probes re-enter the daemon's own ingest as feedback noise
needing nonce suppression, one probe per scanned path amplifies under
enumeration, and one configured origin cannot disambiguate virtual
hosts); observing `416` totals (deferred: error-path traffic is exactly
the noise this design removes); a temporary `SentBytes` fallback for
un-upgraded emitters (rejected: the bug as a migration strategy).

Consequences: sizes are as served - for gzip paths the estimate blends
the compressed and identity representations, while per-representation
increments stay wire-consistent and the increment cap at 1 absorbs the
residual skew; `206` transfers now yield exact totals and count as their
true fraction of a download; rollout must deploy the log format
(`clen`, `crange`) before upgrading bandage.
