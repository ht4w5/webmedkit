package app

import (
	"sync/atomic"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/infra/httpapi"
)

// ruleCounters is the in-memory Rule counter registry: atomic per-rule
// visit and match counts keyed by rule ID in rules-file order, holding
// entries only for counter-enabled rules (GLOSSARY: Rule counter).
// Cumulative since daemon start; restart resets it like every other
// Stats number. It is the internal seam between the evaluator, which
// counts into it, and the daemon, which exposes it - not a public
// contract. The atomics keep the counts exact under the worker pool.
type ruleCounters struct {
	entries []*ruleCounter
	byID    map[string]*ruleCounter
}

// ruleCounter is one enabled rule's counts.
type ruleCounter struct {
	id      string
	total   atomic.Int64
	matched atomic.Int64
}

// newRuleCounters builds the registry from the validated chains: the
// counter-enabled rules in rules-file order (the loader enforces IDs
// unique across the file).
func newRuleCounters(chains []domain.Chain) *ruleCounters {
	rc := &ruleCounters{byID: make(map[string]*ruleCounter)}
	for _, chain := range chains {
		for _, rule := range chain.Rules {
			if !rule.Counter {
				continue
			}
			entry := &ruleCounter{id: rule.ID}
			rc.entries = append(rc.entries, entry)
			rc.byID[rule.ID] = entry
		}
	}
	return rc
}

// Visit counts one rule visit: every event the rule is evaluated
// against (evaluation.Counters). Unknown - counter-disabled - rules
// are a no-op.
func (rc *ruleCounters) Visit(ruleID string) {
	if entry := rc.byID[ruleID]; entry != nil {
		entry.total.Add(1)
	}
}

// Match counts one Match of the rule (evaluation.Counters); a ladder's
// fired rungs count once. Unknown rules are a no-op.
func (rc *ruleCounters) Match(ruleID string) {
	if entry := rc.byID[ruleID]; entry != nil {
		entry.matched.Add(1)
	}
}

// snapshot renders the registry as the /api/stats rules block: one
// {id, total, matched} entry per enabled rule, rules-file order.
func (rc *ruleCounters) snapshot() []httpapi.RuleCounter {
	out := make([]httpapi.RuleCounter, 0, len(rc.entries))
	for _, entry := range rc.entries {
		out = append(out, httpapi.RuleCounter{
			ID:      entry.id,
			Total:   entry.total.Load(),
			Matched: entry.matched.Load(),
		})
	}
	return out
}
