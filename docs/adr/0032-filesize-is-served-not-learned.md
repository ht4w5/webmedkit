# Filesize is served, not learned

The Filesize of an Event was *learned*: a filesize enrichment observed
eligible responses, smoothed them through an EWMA per path and stamped
the estimate onto later Events as a derived field (ADRs 0019, 0029).
The learning model taxed every user of the size signal - learned
records shared the tracker store's budget and could evict live tracker
state, a path's first downloads replay-blind while the estimate warmed,
gzip and identity representations blended into one skewed number, and
the fixed Sampling rule was a second, invisible rule system beside the
Filter. Filesize is now **served, not learned**: the Filesize of an
Event is the file's size as served with that response, derived at parse
time from the fields the log already carries (GLOSSARY: Filesize). The
filesize kind - enricher, EWMA record, cache port and store adapter,
cache namespace and TTL reservation - dies, and the Sampling rule
retires with it. This supersedes the two learning decisions (ADR 0019,
ADR 0029) and amends the two whose rationale bends (ADR 0021, ADR
0024); each carries its trailing note.

The derivation is fixed and total. On a `206` the Content-Range total
is the only size source: a chunk's partial Content-Length must never
masquerade as a file size, or a download resumed across chunks would
shatter into full replay increments - each `206` instead counts its
true fraction of the one download, so a chunked download saturates a
replay ladder once as one download, not once per chunk. An unusable or
absent Content-Range reads as unknown, whatever the Content-Length
says. On any other status the Content-Length serves as Filesize when
greater than zero; absent, empty or zero reads as unknown. There is no
method gating - a `HEAD 200` carrying a Content-Length is a known
size. Parsing keeps its deliberate asymmetry on a new justification: a
malformed Content-Length is a malformed typed field and drops the line
like any invalid field, while an unusual Content-Range is only an odd
free-form header - every other field of the Event is still good - and
degrades to no size, never dropping the Event.

Replay semantics are preserved structurally and reinterpreted
semantically: unknown sizes blind-skip the tracker exactly as before,
and the increment is the transferred fraction of this response's served
size, capped at 1. Zero keeps meaning "unknown" and nothing else -
"known empty" stays unrepresentable and the size Filters keep their
established meaning (ADR 0024 stands on this rationale). The Event
loses the `content_length` and `content_range_total` fields - raw
response-header residue of the learning pipeline - so the report
sample, the log action's `event` attribute and every marshalled Event
lose the two keys while `size_bytes` survives with its redefined
meaning; the whole-event stance stands unchanged (ADR 0021 amended),
and stored documents carrying the removed keys keep parsing, unknown
keys being tolerated by construction (the document stays at `version`
0). With Filesize now parse-time state it moves to the Event's
original-field section, and the derived-field reset at evaluation entry
is deleted: the one-reset convention returns with the first future
enrichment's derived field.

The enrichment plumbing itself survives with an empty kind vocabulary
(ADR 0028 stands): the enrich Action and its rules parsing, the
enrichment and counter seams, the per-(rule, kind) Enrichment counters,
their metric families and the Stats page table are reserved plumbing,
ready for the first future kind. With the vocabulary empty every enrich
action fails load, naming its type and stating that no enrichment types
are available - a leftover `type: filesize` action is caught at load,
loudly and helpfully. Rollout is deliberately un-coupled: the log
format keeps `clen` and `crange`, so upgrading the daemon alone flips
the semantics, and legacy or un-upgraded lines simply read as unknown
sizes - replay stays blind there exactly as before.

Considered: keeping the per-event partial length as the Filesize on a
`206` (rejected: the partial Content-Length masquerades as a file size
and shatters chunked downloads into full replay increments - the
Content-Range total is the same response's accounting of the whole
file, and when it is unusable the honest answer is unknown); keeping
the learned estimate beside - or behind - the served size (rejected:
the cache pollution, the blind first downloads and the representation
blending are properties of learning itself, not of its tuning, and two
size sources would need a reconciliation rule nobody asked for);
removing the enrichment plumbing wholesale with the kind (rejected:
the enrich Action, its seams, the counters and their surfaces are
kind-agnostic machinery, ADR 0028's demand-driven shape stands, and
the alternative re-plumbs all of it when the first kind lands).

Consequences: each response reports its own representation's size, so
gzip and identity traffic no longer blend; there are no cache entries,
no warm-up and no Sampling rule at all, and a path's first download
counts at full weight immediately. The report contract sheds two keys
- canonical example, consumer mirror and goldens updated together - and
rows persisted before the change read back with the keys ignored. The
stale promise of a load error "naming the migration" for removed syntax
dies with the rewrite: removed syntax rides the strict decoder's
generic refusal, as ADR 0028's review note already established.
