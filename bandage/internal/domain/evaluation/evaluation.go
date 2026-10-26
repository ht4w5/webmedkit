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
	// Trigger writes rec for (rule, target). allowOverride marks the
	// write as a ladder rung's Trigger: only such a Trigger may override
	// a live report in place, and only from strictly above the volume it
	// records (the rung's static threshold). A rule without a ladder
	// never overrides its live report - one write per report lifetime
	// (ADR 0013) - so its Trigger passes false.
	Trigger(ctx context.Context, rec domain.Report, allowOverride bool) error
}

// Logger is the log action's logging port (ADR 0005); it carries only the domain's four levels, so a rule can never exit the process.
type Logger interface {
	Log(level domain.Level, msg string, fields map[string]any)
}

// nopLogger discards every record (the Service default).
type nopLogger struct{}

func (nopLogger) Log(domain.Level, string, map[string]any) {}

// Service evaluates events against the loaded chains.
type Service struct {
	chains  []domain.Chain
	byName  map[string]int
	cache   TrackerCache
	reports ReportSink
	now     func() time.Time
	logger  Logger
}

func NewService(chains []domain.Chain, cache TrackerCache, reports ReportSink, now func() time.Time, logger Logger) *Service {
	if logger == nil {
		logger = nopLogger{}
	}
	byName := make(map[string]int, len(chains))
	for i, chain := range chains {
		byName[chain.Name] = i
	}
	return &Service{chains: chains, byName: byName, cache: cache, reports: reports, now: now, logger: logger}
}

// Evaluate runs every chain over the event: chains in file order, rules within a chain in order.
func (s *Service) Evaluate(ctx context.Context, event *domain.Event) error {
	for i := range s.chains {
		if err := s.runChain(ctx, i, event); err != nil {
			return err
		}
	}
	return nil
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
func (s *Service) runChain(ctx context.Context, ci int, event *domain.Event) error {
	chain := &s.chains[ci]
	for ri := range chain.Rules {
		rule := &chain.Rules[ri]
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
		// A fired match resolves to the rungs it fired before any action
		// runs: every rung the post-accumulation value stands above,
		// lowest first (a single-volume rule is one implicit rung). None
		// fired means no match. Dispatch, terminality and the reading all
		// follow the rungs.
		rungs := rule.Rungs(reading)
		if len(rungs) == 0 {
			continue
		}
		if err := s.runRungs(ctx, matchContext{chain: chain, rule: rule, reading: reading, event: event}, rungs); err != nil {
			return err
		}
		if terminal(rungs) {
			break
		}
	}
	return nil
}

// runRungs runs every fired rung's full action list in order, lowest rung
// first; terminality applies only after the lists. The fired rungs were
// decided once from the post-accumulation value before any action ran,
// and the loader's strictly ascending thresholds make them a prefix of
// the ladder - so an action within the event can never cancel a rung that
// already fired. Each rung's actions see its trigger-time reading: the
// reading captured before any action ran, carrying the rung's threshold
// as its volume.
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

// terminal reports whether a fired rung ends chain traversal after the
// rule's rungs have run: a rung whose action list carries report, goto or
// stop (the rung's Terminal). A fired jump never ends the chain; a fired
// goto ran its target chain and ends this chain here, after the
// remaining fired rungs have run.
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

// updateTracker feeds the rule's tracker (when it has one) and reports
// the post-accumulation reading. fed is false only for replay warm-up
// (unknown sizes are skipped entirely); tracker-less rules are always
// fed. Whether the rule matched is the fired rungs' question (Rule.Rungs).
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
			// Warm-up blindness: skip unknown sizes entirely - no increment,
			// no decay, no cache touch.
			return domain.TrackerReading{}, false, nil
		}
		// Increment is the transferred fraction of the estimated size, capped
		// at 1 per event.
		size := float64(event.SizeBytes)
		increment = math.Min(float64(event.SentBytes)/size, 1)
		// Volume is the volume method's output for this event's estimate;
		// the cache-entry TTL is re-derived from it per event. A ladder's
		// scalar rungs stand in for the volume - download equivalents,
		// already normalized by Filesize - so the TTL re-derives from the
		// top rung and state outlives the whole climb.
		volume = cfg.VolumeFor(size)
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

// trigger runs the rule's report action: a Trigger suppresses or writes, and never empties the matched rule's tracker record (ADR 0013) - report aliveness alone bounds the writes. Only a ladder rung's Trigger may override the live report in place (its static threshold rides the reading's volume); a rule without a ladder writes at most once per report lifetime. A failed write is not a Trigger outcome (ADR 0008): the record stays intact and the next matching event retries.
func (s *Service) trigger(ctx context.Context, m matchContext, action *domain.ReportAction) error {
	now := s.now()
	penalty := action.Penalty // the stance of the rules loaded at write time
	rec := domain.Report{
		Rule:      m.rule.ID,
		Chain:     m.chain.Name,
		Reason:    action.Reason,
		Target:    m.target(),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(action.TTL),
		Tracker:   m.reading,
		Sample: domain.Sample{
			Time:   m.event.Time,
			Path:   m.event.Path,
			Agent:  m.event.Agent,
			Status: m.event.Status,
			Sent:   m.event.SentBytes,
			Size:   m.event.SizeBytes,
		},
		Penalty: &penalty,
	}
	return s.reports.Trigger(ctx, rec, len(m.rule.Escalations) > 0)
}
