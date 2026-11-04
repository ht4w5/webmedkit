package app

import (
	"sync/atomic"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/infra/httpapi"
)

// enrichmentCounters is the in-memory Enrichment counter registry:
// atomic per-(rule, enrichment) run counts keyed by the pair in
// rules-file order of pairs, holding entries only for counter-enabled
// pairs (GLOSSARY: Enrichment counter). A pair is enabled when any of
// the rule's enrich actions of that kind carries counter: true, and
// every run of that kind at the rule counts toward its one row.
// Cumulative since daemon start; restart resets it like every other
// Stats number. It is the internal seam between the evaluator, which
// counts into it, and the daemon, which exposes it - not a public
// contract. The atomics keep the counts exact under the worker pool.
type enrichmentCounters struct {
	entries []*enrichmentCounter
	byPair  map[string]*enrichmentCounter
}

// enrichmentCounter is one (rule, enrichment) pair's counts; pairs
// that no rule counter-enabled keep their position but never count.
type enrichmentCounter struct {
	rule       string
	enrichment string
	enabled    bool
	total      atomic.Int64
	observed   atomic.Int64
	enriched   atomic.Int64
}

// pairKey keys the registry by (rule ID, enrichment kind); the loader
// enforces rule IDs unique across the file.
func pairKey(ruleID, kind string) string {
	return ruleID + "\x00" + kind
}

// newEnrichmentCounters builds the registry from the validated chains:
// one entry per (rule, enrichment) pair in rules-file order of pairs -
// first occurrence across the rule's action lists, ladder rungs
// included (rule.ActionLists()) - enabled when any of the rule's
// enrich actions of that kind carries counter: true.
func newEnrichmentCounters(chains []domain.Chain) *enrichmentCounters {
	ec := &enrichmentCounters{byPair: make(map[string]*enrichmentCounter)}
	for _, chain := range chains {
		for _, rule := range chain.Rules {
			for _, list := range rule.ActionLists() {
				for _, action := range list {
					if action.Enrich == nil {
						continue
					}
					entry := ec.entry(rule.ID, action.Enrich.Type)
					entry.enabled = entry.enabled || action.Enrich.Counter
				}
			}
		}
	}
	return ec
}

// entry returns the pair's entry, created at its first occurrence.
func (ec *enrichmentCounters) entry(ruleID, kind string) *enrichmentCounter {
	key := pairKey(ruleID, kind)
	if e := ec.byPair[key]; e != nil {
		return e
	}
	e := &enrichmentCounter{rule: ruleID, enrichment: kind}
	ec.entries = append(ec.entries, e)
	ec.byPair[key] = e
	return e
}

// Run counts one enrichment run of kind at ruleID with the run's
// outcomes (evaluation.EnrichmentCounters); a failed run counts with
// outcomes false (the run happened however it ended). Unknown -
// counter-disabled - pairs are a no-op.
func (ec *enrichmentCounters) Run(ruleID, kind string, observed, enriched bool) {
	e := ec.byPair[pairKey(ruleID, kind)]
	if e == nil || !e.enabled {
		return
	}
	e.total.Add(1)
	if observed {
		e.observed.Add(1)
	}
	if enriched {
		e.enriched.Add(1)
	}
}

// snapshot renders the registry as the /api/stats enrichment block:
// one {rule, enrichment, total, observed, enriched} entry per enabled
// pair, rules-file order of pairs.
func (ec *enrichmentCounters) snapshot() []httpapi.EnrichmentCounter {
	out := make([]httpapi.EnrichmentCounter, 0, len(ec.entries))
	for _, e := range ec.entries {
		if !e.enabled {
			continue
		}
		out = append(out, httpapi.EnrichmentCounter{
			Rule:       e.rule,
			Enrichment: e.enrichment,
			Total:      e.total.Load(),
			Observed:   e.observed.Load(),
			Enriched:   e.enriched.Load(),
		})
	}
	return out
}
