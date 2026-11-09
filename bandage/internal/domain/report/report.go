// Package report holds the report lifecycle: the Service every report mutation
// flows through (Trigger write, delete-by-filter, expiry deletion) and its port.
package report

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Query filters reports by rule and/or target (empty = no filter).
type Query struct {
	Rule   string
	Target string
	// Now bounds expiry: fixed rows with expires_at <= Now are never
	// returned (dynamic rows are bounded by their tracker value).
	Now time.Time
}

// Matches reports whether a (rule, target) pair falls in the query's filter: an empty Rule or Target matches everything.
func (q Query) Matches(rule, target string) bool {
	return (q.Rule == "" || q.Rule == rule) && (q.Target == "" || q.Target == target)
}

// ErrUnfiltered is returned by Repository.Delete when neither Rule nor Target is set: deletion is never delete-all.
var ErrUnfiltered = errors.New("report: delete requires a rule or target filter")

// Repository persists reports, one row per (rule, target).
type Repository interface {
	// Upsert inserts or overwrites the report for (rule, target); created_at
	// stays fixed at the row's first insert.
	Upsert(ctx context.Context, rec domain.Report) error
	// List returns matching, live reports sorted by created_at descending:
	// unexpired fixed reports plus every dynamic report (spec: Persistence).
	List(ctx context.Context, q Query) ([]domain.Report, error)
	// Delete removes reports matching the query and returns the deleted row count.
	// Rule or Target must be set (ErrUnfiltered otherwise); rows match regardless of expiry.
	Delete(ctx context.Context, q Query) (int, error)
	// DeleteExpired deletes fixed rows whose expires_at has passed and
	// returns the number of deleted rows; dynamic rows die by value,
	// never by time (spec: Persistence).
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
	// DeleteDynamic deletes every dynamic report and returns the number
	// of deleted rows - the startup sweep (spec: Startup sweep).
	DeleteDynamic(ctx context.Context) (int, error)
}

// TrackerLookup reads tracker state: the liveness seam dynamic reports
// die against (spec: Dynamic report liveness). Absent and expired
// records read as the zero record - no accumulation, dead.
type TrackerLookup interface {
	Get(ctx context.Context, ns, key string) domain.Record
}

// NopTrackerLookup reads no tracker state: every record reads empty.
// It is the TrackerLookup for callers with no dynamic reports (fixed
// reports never consult it), passed explicitly - nothing silently
// substitutes for a missing dependency.
type NopTrackerLookup struct{}

func (NopTrackerLookup) Get(context.Context, string, string) domain.Record { return domain.Record{} }

// Service is the one report lifecycle module: Trigger upserts one report per (rule, target)
// and escalates it in place - only a strictly higher ladder rung overrides a live report; every other Trigger writes nothing (ADR 0013).
type Service struct {
	repo     Repository
	trackers TrackerLookup

	mu   sync.Mutex
	live map[reportKey]*liveEntry
}

// reportKey is a report row's identity: (rule, target).
type reportKey struct {
	rule   string
	target string
}

// liveEntry is one (rule, target)'s suppression state. Aliveness is
// mode-aware (spec: Trigger suppression): fixed entries die at their
// settled expiry, dynamic entries when the tracker record behind them
// underflows the recorded volume - the threshold an override must
// stand strictly above either way.
type liveEntry struct {
	dynamic   bool
	expiresAt time.Time // fixed entries: the settled expiry
	volume    float64   // recorded threshold an override must stand strictly above
	margin    float64   // the writing action's grace margin: the death line is volume - margin
	leak      float64   // dynamic entries: the reading's decay rate
	key       string    // dynamic entries: the writing tracker key (ns is the rule)
	// inflight is non-nil only while this key's write is in flight; closed under
	// s.mu once settled (kept on success, rolled back to the prior entry on failure).
	inflight chan struct{}
}

func NewService(repo Repository, trackers TrackerLookup) *Service {
	return &Service{repo: repo, trackers: trackers, live: make(map[reportKey]*liveEntry)}
}

// alive reports whether the entry's report is alive at now: fixed
// entries by their expiry (unchanged), dynamic entries by value-based
// liveness against the tracker record of their writing key (spec:
// Trigger suppression).
func (s *Service) alive(ctx context.Context, rule string, e *liveEntry, now time.Time) bool {
	if e.dynamic {
		rec := s.trackers.Get(ctx, rule, e.key)
		return !rec.Underflow(e.volume-e.margin, e.leak, now.Unix())
	}
	return e.expiresAt.After(now)
}

// Trigger records the report for (rule, target). A failed write is not a Trigger
// outcome: the record stands and the next matching event retries (ADR 0013).
func (s *Service) Trigger(ctx context.Context, rec domain.Report, allowOverride bool) error {
	key := reportKey{rule: rec.Rule, target: rec.Target}
	var e, prev *liveEntry
	for {
		s.mu.Lock()
		cur, ok := s.live[key]
		if ok && cur.inflight != nil {
			// A write for this key is pending: wait for its outcome, then
			// re-evaluate (the pending write is not a live report).
			ch := cur.inflight
			s.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return fmt.Errorf("trigger report: %w", ctx.Err())
			}
		}
		if ok && s.alive(ctx, key.rule, cur, rec.UpdatedAt) && (!allowOverride || rec.Tracker.Volume <= cur.volume) {
			s.mu.Unlock()
			return nil
		}
		prev = cur
		e = &liveEntry{inflight: make(chan struct{})}
		s.live[key] = e
		s.mu.Unlock()
		break
	}
	if err := s.repo.Upsert(ctx, rec); err != nil {
		s.mu.Lock()
		if s.live[key] == e {
			if prev != nil {
				// The failed write leaves the old row standing: its entry keeps suppressing.
				s.live[key] = prev
			} else {
				delete(s.live, key)
			}
		}
		ch := e.inflight
		e.inflight = nil
		close(ch)
		s.mu.Unlock()
		return fmt.Errorf("trigger report: %w", err)
	}
	s.mu.Lock()
	e.dynamic = rec.Dynamic
	e.expiresAt = rec.ExpiresAt
	e.volume = rec.Tracker.Volume
	e.margin = rec.Margin
	e.leak = rec.Tracker.Leak
	e.key = writingKey(rec)
	ch := e.inflight
	e.inflight = nil
	close(ch)
	s.mu.Unlock()
	return nil
}

// writingKey derives the tracker-cache key of the report's reading.
func writingKey(rec domain.Report) string {
	_, key := rec.WritingKey()
	return key
}

// Rebuild derives the live-report set from the report table at now and
// supplants the set, keeping in-flight entries; runs under s.mu (no
// interleaving). Only fixed rows seed the set: a dynamic report cannot
// justify suppression across a restart, where the volatile tracker
// state behind it starts empty (spec: Startup sweep).
func (s *Service) Rebuild(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.repo.List(ctx, Query{Now: now})
	if err != nil {
		return err
	}
	live := make(map[reportKey]*liveEntry, len(rows))
	for _, rec := range rows {
		if rec.Dynamic {
			continue
		}
		live[reportKey{rule: rec.Rule, target: rec.Target}] = &liveEntry{expiresAt: rec.ExpiresAt, volume: rec.Tracker.Volume}
	}
	for key, e := range s.live {
		if e.inflight != nil {
			live[key] = e
		}
	}
	s.live = live
	return nil
}

// List returns matching, alive reports (Repository.List). A dynamic
// report whose tracker value has underflowed is dead at document
// generation: it is never served, and its live-set entry dies with it,
// so the next overflow writes (spec: Dynamic report liveness).
func (s *Service) List(ctx context.Context, q Query) ([]domain.Report, error) {
	rows, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Report, 0, len(rows))
	var dead []reportKey
	for _, rec := range rows {
		if rec.Dynamic {
			ns, key := rec.WritingKey()
			if s.trackers.Get(ctx, ns, key).Underflow(rec.Tracker.Volume-rec.Margin, rec.Tracker.Leak, q.Now.Unix()) {
				dead = append(dead, reportKey{rule: rec.Rule, target: rec.Target})
				continue
			}
		}
		out = append(out, rec)
	}
	if len(dead) > 0 {
		s.mu.Lock()
		for _, key := range dead {
			if e := s.live[key]; e != nil && e.inflight == nil {
				delete(s.live, key)
			}
		}
		s.mu.Unlock()
	}
	return out, nil
}

// Delete removes reports matching the query and returns the number of deleted
// rows (Repository.Delete). The deleted pairs leave the live set: re-arms writing.
func (s *Service) Delete(ctx context.Context, q Query) (int, error) {
	n, err := s.repo.Delete(ctx, q)
	if err != nil {
		return n, err
	}
	s.mu.Lock()
	for key := range s.live {
		if q.Matches(key.rule, key.target) {
			delete(s.live, key)
		}
	}
	s.mu.Unlock()
	return n, nil
}

// DeleteExpired deletes the reports whose life has ended at now and
// returns the number of deleted rows: fixed rows past their expiry
// (Repository.DeleteExpired) and dynamic rows whose tracker value has
// underflowed - the lazy death materialized at sweep time (spec:
// Dynamic report liveness). Swept reports leave the live set.
func (s *Service) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	n, err := s.repo.DeleteExpired(ctx, now)
	if err != nil {
		return n, err
	}
	rows, err := s.repo.List(ctx, Query{Now: now})
	if err != nil {
		return n, err
	}
	for _, rec := range rows {
		if !rec.Dynamic {
			continue
		}
		ns, key := rec.WritingKey()
		if !s.trackers.Get(ctx, ns, key).Underflow(rec.Tracker.Volume-rec.Margin, rec.Tracker.Leak, now.Unix()) {
			continue
		}
		m, err := s.repo.Delete(ctx, Query{Rule: rec.Rule, Target: rec.Target})
		if err != nil {
			return n, err
		}
		n += m
	}
	s.mu.Lock()
	for key, e := range s.live {
		if e.inflight != nil {
			continue
		}
		if e.dynamic {
			rec := s.trackers.Get(ctx, key.rule, e.key)
			if !rec.Underflow(e.volume-e.margin, e.leak, now.Unix()) {
				continue
			}
		} else if e.expiresAt.After(now) {
			continue
		}
		delete(s.live, key)
	}
	s.mu.Unlock()
	return n, nil
}

// DeleteDynamic deletes every dynamic report and returns the number of
// deleted rows - the startup sweep (spec: Startup sweep): no Report
// outlives the volatile tracker state that justifies it. The deleted
// dynamic pairs leave the live set; fixed entries are untouched.
func (s *Service) DeleteDynamic(ctx context.Context) (int, error) {
	n, err := s.repo.DeleteDynamic(ctx)
	if err != nil {
		return n, err
	}
	s.mu.Lock()
	for key, e := range s.live {
		if e.dynamic && e.inflight == nil {
			delete(s.live, key)
		}
	}
	s.mu.Unlock()
	return n, nil
}
