# 01: Served-size Filesize from the parser

**What to build:** The Filesize of an Event becomes the file's size as
served with that response, derived while the access-log record is
parsed - per the spec at `.scratch/filesize-served-not-learned/spec.md`.
On a 206 the Content-Range total is the only size source (an unusable
one reads as unknown even when a Content-Length is present); any other
response takes its Content-Length when greater than zero, else unknown;
no method gating. The strict-Content-Length / lenient-Content-Range
validation asymmetry stays. Filesize is parse-time state, so the
evaluation-entry derived-field reset dies with it. The filesize
enrichment and all of its learned state are deleted; the enrichment
kind vocabulary is empty and any enrich Action fails load naming the
type and stating that no enrichment types are available. Replay
reinterprets the served size (a 206 counts its true fraction of the
download, a full response counts 1, unknown sizes blind-skip as
before); size Filters keep zero-means-unknown on the new meaning.
Shipped and example rules files drop their filesize enrich rules.
The Glossary redefines Filesize as served size and retires the
Sampling rule; the ingest, rules and metrics documents move to the
served-size story, including the stale "load error names the
migration" correction.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Filesize is derived at parse time: 206 uses the Content-Range
      total only; any other status uses the Content-Length when > 0;
      otherwise unknown; no method or status gating beyond the 206 rule
- [ ] A 206 with a Content-Length but no usable Content-Range total
      reads as unknown - partial lengths never become Filesize
- [ ] A malformed Content-Length drops the line like any invalid
      field; an unusual Content-Range degrades to unknown and never
      drops the line
- [ ] Filesize is original parse-time state on the Event; the
      evaluation-entry derived-field reset is gone
- [ ] The filesize enrichment and every trace of learned state (EWMA
      record, cache port and store adapter, window constant, cache
      namespace) are deleted
- [ ] The enrichment kind vocabulary is empty; every enrich Action
      fails load with an error naming the type and "no enrichment
      types are available"; the enrich action stays valid syntax
- [ ] Replay blind-skips unknown sizes exactly as before; a 206
      transfer counts its true fraction of the file; a full response
      counts 1; increments stay capped at 1
- [ ] `size >=`, `size <=` and `size unknown` keep their established
      meaning over the served size (zero = unknown, nothing else)
- [ ] Shipped and example rules files carry no enrich rules and load
      cleanly; size predicates work with no enrichment in the file
- [ ] Glossary defines Filesize as served size and carries no Sampling
      rule; ingest, rules and metrics documents tell the served-size
      story and no longer teach the enrich-rule composition
- [ ] Behavior lands at the existing seams: the assembled-daemon
      end-to-end suite covers the derivation outcomes, chunked-download
      fractions and load errors; the parser field-contract tests cover
      the derivation table and validation asymmetry; suite passes
      race-enabled

## Comments
