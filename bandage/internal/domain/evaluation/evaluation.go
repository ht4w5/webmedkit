// Package evaluation runs the rule model over events: prefilter, tracker
// update, match (filter and overflow) and rung dispatch.
package evaluation

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// TrackerCache is the tracker-state port: records keyed by (ns, key) with sliding TTL and atomic per-key updates. Nothing empties a record: a Trigger suppresses or writes, never resets (ADR 0013).
type TrackerCache interface {
	// Update applies fn to the record for (ns, key) under the store's per-key lock; fn must not do I/O or use ctx. ttl 0 means no expiry.
	Update(ctx context.Context, ns, key string, ttl time.Duration, fn func(*domain.Record)) (domain.Record, error)
}

// ReportSink is the report write path: a Trigger upserts one report per (rule, target).
type ReportSink interface {
	// Trigger writes rec for (rule, target); allowOverride marks a ladder rung's write.
	Trigger(ctx context.Context, rec domain.Report, allowOverride bool) error
}

// Counters is the rule-counter port: per-rule visit and match
// accounting at the evaluator's counting points (GLOSSARY: Rule
// counter). The daemon owns the registry behind it and exposes the
// counts - the registry is an internal seam between evaluator and
// daemon, not a public contract. Implementations must be exact under
// the worker pool.
type Counters interface {
	// Visit counts one rule visit: every event the rule is evaluated
	// against, filter rejects and replay's unknown-size skips included; a
	// chain dispatched more than once for one event counts one visit
	// per dispatch.
	Visit(ruleID string)
	// Match counts one Match of the rule: once per matching Event
	// however many ladder rungs fired.
	Match(ruleID string)
}

// NopCounters discards every count: the Counters implementation for
// callers that observe nothing, passed explicitly - nothing silently
// substitutes for a missing dependency.
type NopCounters struct{}

func (NopCounters) Visit(string) {}
func (NopCounters) Match(string) {}

// Enrichments is the enrichment seam: the global, static enrichment
// instances - one per kind, shared by every enrich action naming it -
// invoked by kind name where an enrich action runs (ADR 0028). The
// daemon owns the instances behind it; the evaluator owns the
// invocation point. Implementations report the run's outcomes for the
// Enrichment counter (GLOSSARY): observed is true when the run fed the
// kind's learned state, enriched true when it produced a non-default
// derived value.
type Enrichments interface {
	// Enrich runs the named enrichment for the event, setting its
	// derived fields. Nothing memoizes per event: one event may run one
	// kind several times, observing each time (ADR 0028).
	Enrich(ctx context.Context, kind string, event *domain.Event) (observed, enriched bool, err error)
}

// NopEnrichments runs no enrichment and observes nothing: the
// Enrichments implementation for callers without enrich actions,
// passed explicitly - nothing silently substitutes for a missing
// dependency.
type NopEnrichments struct{}

func (NopEnrichments) Enrich(context.Context, string, *domain.Event) (bool, bool, error) {
	return false, false, nil
}

// EnrichmentCounters is the Enrichment-counter port: per (rule,
// enrichment) run accounting at the evaluator's counting points
// (GLOSSARY: Enrichment counter). The daemon owns the registry behind
// it and exposes the counts - the registry is an internal seam between
// evaluator and daemon, not a public contract. Implementations must be
// exact under the worker pool.
type EnrichmentCounters interface {
	// Run counts one enrichment run of kind at ruleID - one per
	// invocation, a failed run included (the run happened however it
	// ended) - with the run's outcomes.
	Run(ruleID, kind string, observed, enriched bool)
}

// NopEnrichmentCounters discards every count: the EnrichmentCounters
// implementation for callers that observe nothing, passed explicitly -
// nothing silently substitutes for a missing dependency.
type NopEnrichmentCounters struct{}

func (NopEnrichmentCounters) Run(string, string, bool, bool) {}

// Logger is the log action's logging port (ADR 0005); it carries only the domain's four levels, so a rule can never exit the process.
type Logger interface {
	Log(level domain.Level, msg string, fields map[string]any)
}

// NopLogger discards every record: the Logger implementation for
// callers that observe nothing, passed explicitly - nothing silently
// substitutes for a missing dependency.
type NopLogger struct{}

func (NopLogger) Log(domain.Level, string, map[string]any) {}

// Service evaluates events against the loaded chains.
type Service struct {
	reasonPrefix string
	chains       []domain.Chain
	byName       map[string]int
	cache        TrackerCache
	reports      ReportSink
	counters     Counters
	enrichments  Enrichments
	enrichCount  EnrichmentCounters
	now          func() time.Time
	logger       Logger
}

// Deps are the Service's dependencies and settings, assembled once at
// daemon assembly or a test fixture: one named struct rather than a
// positional argument list, so the prefix and the ports can never
// swap places. Counters, Enrichments, EnrichmentCounters and Logger
// are required - pass their Nop implementations to discard their
// output.
type Deps struct {
	// ReasonPrefix namespaces report reasons at creation (ADR 0016):
	// the rules file's reason_prefix, else the program name (ADR 0026).
	ReasonPrefix string
	// Chains are the loaded chains; the entry chain is domain.EntryChain.
	Chains []domain.Chain
	// Cache is the tracker-state port and Reports the report write port.
	Cache   TrackerCache
	Reports ReportSink
	// Counters receives the rule-counter counts (GLOSSARY: Rule
	// counter); the daemon owns the registry behind it.
	Counters Counters
	// Enrichments runs the named enrichments at enrich actions (ADR
	// 0028); the daemon owns the global kind instances behind it.
	Enrichments Enrichments
	// EnrichmentCounters receives the Enrichment-counter counts
	// (GLOSSARY: Enrichment counter); the daemon owns the registry
	// behind it.
	EnrichmentCounters EnrichmentCounters
	// Now is the clock report timestamps read at Trigger time.
	Now func() time.Time
	// Logger is the log action's logging port (ADR 0005).
	Logger Logger
}

// NewService assembles the evaluator from its dependencies.
func NewService(d Deps) *Service {
	byName := make(map[string]int, len(d.Chains))
	for i, chain := range d.Chains {
		byName[chain.Name] = i
	}
	return &Service{reasonPrefix: d.ReasonPrefix, chains: d.Chains, byName: byName, cache: d.Cache, reports: d.Reports, counters: d.Counters, enrichments: d.Enrichments, enrichCount: d.EnrichmentCounters, now: d.Now, logger: d.Logger}
}

// Evaluate enters the entry chain for the event; every other chain runs
// only when a goto/jump action dispatches to it.
func (s *Service) Evaluate(ctx context.Context, event *domain.Event) error {
	// The single derived-field reset (ADR 0019): once per event at
	// evaluation entry, before any rule runs - not per chain dispatch.
	resetDerived(event)
	ci, ok := s.byName[domain.EntryChain]
	if !ok {
		return fmt.Errorf("evaluate: entry chain %q not found", domain.EntryChain)
	}
	return s.runChain(ctx, ci, event)
}

// resetDerived resets every derived Event field to its default - the
// single reset reserved for future enrichments, so none can leak stale
// state (ADR 0019).
func resetDerived(event *domain.Event) {
	event.SizeBytes = domain.FilesizeUnknown
}

// matchContext is one fired rung in progress: rule, chain, event and the
// trigger-time tracker reading (the rung's threshold as its volume).
type matchContext struct {
	chain   *domain.Chain
	rule    *domain.Rule
	reading domain.TrackerReading
	event   *domain.Event
}

// target is the report Target and tracker key base: the event's source aggregated to the rule tracker's prefix.
func (m matchContext) target() string {
	return m.rule.Tracker.TargetString(m.event.RemoteAddr)
}

// runChain traverses one chain's rules in order; also entered by name from goto/jump actions.
// A Rule counter counts one visit per rule per traversal (a chain
// dispatched more than once for one event counts one visit per
// dispatch) and one Match per matching Event (GLOSSARY: Rule counter).
func (s *Service) runChain(ctx context.Context, ci int, event *domain.Event) error {
	chain := &s.chains[ci]
	for ri := range chain.Rules {
		rule := &chain.Rules[ri]
		s.counters.Visit(rule.ID) // total: filter rejects and replay's unknown-size skips are visits too
		if !rule.Filter.Match(event) {
			continue
		}
		reading, fed, err := s.updateTracker(ctx, rule, event)
		if err != nil {
			return err
		}
		if !fed {
			continue
		}
		rungs := rule.Rungs(reading)
		if len(rungs) == 0 {
			continue
		}
		s.counters.Match(rule.ID) // matched: once per Match, a ladder's fired rungs count once
		if err := s.runRungs(ctx, matchContext{chain: chain, rule: rule, reading: reading, event: event}, rungs); err != nil {
			return err
		}
		if terminal(rungs) {
			break
		}
	}
	return nil
}

// runRungs runs every fired rung's full action list in order, lowest rung first.
// The rungs are decided before any action runs and form a ladder prefix, so no action cancels one.
func (s *Service) runRungs(ctx context.Context, m matchContext, rungs []domain.Escalation) error {
	for _, rung := range rungs {
		m.reading.Volume = rung.At
		dispatch := actionDispatch{svc: s, ctx: ctx, m: m}
		for _, action := range rung.Actions {
			if err := action.Accept(&dispatch); err != nil {
				return err
			}
		}
	}
	return nil
}

// terminal reports whether a fired rung ends chain traversal after the rule's rungs
// have run; a fired report or jump never ends the chain (a report writes and the
// event continues through the chain's remaining rules, ADR 0027), a fired goto
// only after the remaining rungs.
func terminal(rungs []domain.Escalation) bool {
	for _, rung := range rungs {
		if rung.Terminal() {
			return true
		}
	}
	return false
}

// actionDispatch is the evaluation side of Action.Accept: it executes one action variant for a match.
type actionDispatch struct {
	svc *Service
	ctx context.Context
	m   matchContext
}

func (d *actionDispatch) VisitReport(a *domain.ReportAction) error {
	return d.svc.trigger(d.ctx, d.m, a)
}

func (d *actionDispatch) VisitLog(a *domain.LogAction) error {
	// The record carries the rule, its Target, the event and the reading.
	d.svc.logger.Log(a.Level, a.Message, map[string]any{
		"rule":    d.m.rule.ID,
		"target":  d.m.target(),
		"event":   d.m.event,
		"tracker": d.m.reading,
	})
	return nil
}

func (d *actionDispatch) VisitEnrich(a *domain.EnrichAction) error {
	observed, enriched, err := d.svc.enrichments.Enrich(d.ctx, a.Type, d.m.event)
	// The counting point: one run per invocation with the run's
	// outcomes - no memoization, so one event may run one kind twice
	// (GLOSSARY: Enrichment counter, ADR 0028).
	d.svc.enrichCount.Run(d.m.rule.ID, a.Type, observed, enriched)
	return err
}

func (d *actionDispatch) VisitGoto(target string) error {
	return d.svc.runNamedChain(d.ctx, d.m.rule, target, d.m.event)
}

func (d *actionDispatch) VisitJump(target string) error {
	return d.svc.runNamedChain(d.ctx, d.m.rule, target, d.m.event)
}

func (d *actionDispatch) VisitStop() error { return nil }

// runNamedChain runs a goto/jump target chain by name (the loader has validated that the target exists).
func (s *Service) runNamedChain(ctx context.Context, rule *domain.Rule, name string, event *domain.Event) error {
	ci, ok := s.byName[name]
	if !ok {
		return fmt.Errorf("evaluate rule %q: target chain %q not found", rule.ID, name)
	}
	return s.runChain(ctx, ci, event)
}

// updateTracker feeds the rule's tracker (when it has one) and reports the
// post-accumulation reading. fed is false only for replay with an unknown size (a never-observed path).
func (s *Service) updateTracker(ctx context.Context, rule *domain.Rule, event *domain.Event) (domain.TrackerReading, bool, error) {
	if rule.Tracker == nil {
		// Tracker-less rules match on filter alone.
		return domain.TrackerReading{}, true, nil
	}
	cfg := rule.Tracker
	increment := 1.0
	volume := cfg.Volume
	ttl := cfg.TTL
	key := matchKey(cfg, event)
	switch cfg.Kind {
	case domain.TrackBandwidth:
		increment = float64(event.SentBytes)
	case domain.TrackReplay:
		if event.SizeBytes == domain.FilesizeUnknown {
			// Blind skip of unknown sizes (a never-observed path): no
			// increment, no decay, no cache touch.
			return domain.TrackerReading{}, false, nil
		}
		// Increment is the transferred fraction of the estimated size, capped
		// at 1 per event.
		size := float64(event.SizeBytes)
		increment = math.Min(float64(event.SentBytes)/size, 1)
		// The reading's volume is the rule's constant scalar; a ladder's
		// rungs stand in for the volume, so its TTL derives from the top
		// rung. The cache-entry TTL re-derives from the volume per event.
		if top, ok := rule.TopRung(); ok {
			volume = top.At
		}
		ttl = domain.TTL(cfg.Leak, volume)
	}
	rec, err := s.cache.Update(ctx, rule.ID, key, ttl, func(r *domain.Record) {
		r.Accumulate(increment, cfg.Leak, event.Time.Unix())
	})
	if err != nil {
		return domain.TrackerReading{}, false, fmt.Errorf("evaluate rule %q: %w", rule.ID, err)
	}
	reading := domain.TrackerReading{
		Track:  string(cfg.Kind),
		Value:  rec.Value,
		Leak:   cfg.Leak,
		Volume: volume,
	}
	return reading, true, nil
}

// matchKey derives the tracker cache key: the Target, or for replay the Target and path joined with KeySep.
func matchKey(cfg *domain.TrackerConfig, event *domain.Event) string {
	target := cfg.TargetString(event.RemoteAddr)
	if cfg.Kind == domain.TrackReplay {
		return domain.ReplayTrackerKey(target, event.Path)
	}
	return domain.TrackerKey(target)
}

func (s *Service) trigger(ctx context.Context, m matchContext, action *domain.ReportAction) error {
	now := s.now()
	penalty := action.Penalty // the stance of the rules loaded at write time
	// A report action without ttl writes a dynamic report: no scheduled
	// expiry, alive while the tracker value stays above the recorded
	// volume (spec: Two lifecycle modes).
	dynamic := action.TTL == 0
	expires := now.Add(action.TTL)
	if dynamic {
		expires = time.Time{}
	}
	rec := domain.Report{
		Rule:      m.rule.ID,
		Chain:     m.chain.Name,
		Reason:    domain.NamespacedReason(s.reasonPrefix, action.Reason), // namespaced at creation only (ADR 0016)
		Target:    m.target(),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: expires,
		Tracker:   m.reading,
		Sample:    *m.event, // the Event whole (ADR 0021), never a projection
		Penalty:   &penalty,
		Dynamic:   dynamic,
	}
	return s.reports.Trigger(ctx, rec, len(m.rule.Escalations) > 0)
}
