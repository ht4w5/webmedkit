# Enrichment is demand-driven, run by rule actions

Enrichment ran once per event before evaluation, gated by a rules-file
filter over the whole path. The Filesize estimate's records share the
tracker store - same entry cap, same per-shard LRU eviction - so traffic
the enrichment learned from but no rule ever consumed (bursts of
first-seen paths) churned entries and evicted live tracker state.
Enrichment is therefore an Action in the evaluation pipeline (`enrich`),
executed where a rule matches, its derived fields consumed by subsequent
rules: the rules file is the single source of truth for which events get
enriched, since an enrich action's rule filter defines the demand domain
and cannot drift from the rules the way a duplicated scoping predicate
would. The enrichments themselves are global and static - one instance
per enrichment kind, shared by every enrich action naming it (`type:
filesize`), with no per-action instance or configuration - and each
`enrich` action carries its own `counter` opt-in, counting that
(rule, enrichment) pair's runs (GLOSSARY: Enrichment counter).

Considered options: keeping the pre-evaluation path with per-enricher
filter blocks (rejected: the scoping predicate duplicates what rule
filters already say and drifts from them, and no static predicate can
see evaluation state); bounding only the cache instead of moving
enrichment (rejected as the sole fix: an entry cap limits the blast
radius but still creates entries nothing consumes - kept as follow-up
`.scratch/v0-followups/issues/06`, since a legitimate-but-huge path set
can still compete with tracker records); a special enrich phase before
tracker updates (rejected: it breaks the uniform "an Action runs when a
rule matches" semantics - the documented composition is a tracker-less
enrich rule ahead of the consuming rule).

Consequences: an enrich action runs after its own rule's tracker update,
so it feeds subsequent rules only; a replay tracker is fed by an enrich
action in an earlier rule. Nothing memoizes enrichment per event: one
event may run an enrichment twice (two enrich actions, or a chain
dispatched twice per event), observing twice and skewing the estimate -
preventing that is the rules author's responsibility, and Enrichment
counters count runs, not events. Observation becomes demand-driven too:
an eligible response contributes an observation only when its event
reaches an enrich action, so terminal actions upstream starve the
estimate on that path (accepted; GLOSSARY: Sampling rule). The rules
file's top-level `enrichment` block and its filter are gone; the old
shape fails load with a pointed error.
