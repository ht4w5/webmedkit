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

// Stance is one ladder rung's report stance - what a Downgrade
// restates a decaying report to (spec: Downgrade). It is the rung's
// report action in restated form: the policy is always the writing
// action's, and the reason is already namespaced as at write time
// (ADR 0016).
type Stance struct {
	// At is the rung's threshold: the restated reading's volume, the
	// base of the death line, and the guard an override must stand
	// strictly above.
	At float64
	// Reason is the rung's report reason in namespaced form.
	Reason string
	// Penalty is the rung's advisory enforcement stance (ADR 0010).
	Penalty domain.Penalty
	// TTL is the rung's policy: a fixed rung's ttl, stamped from the
	// downgrade write; zero means a dynamic stance (Dynamic) - alive
	// while the decayed value stays strictly above At minus Margin.
	TTL time.Duration
	// Margin is the rung's grace margin (leak x grace); the dynamic
	// stance's death line is At minus Margin. Zero for fixed stances
	// (ttl and grace are mutually exclusive).
	Margin float64
}

// Dynamic reports the stance's mode: a fixed rung carries its ttl, a
// dynamic rung none - the policy is always the writing action's (spec:
// Two lifecycle modes).
func (st Stance) Dynamic() bool { return st.TTL == 0 }

// Ladder resolves the report stances of one rule's ladder, lowest rung
// first (spec: Downgrade). Rungs without a report action carry no
// stance - there is nothing to restate to. A rule without a ladder
// resolves to nothing: a single-volume report has no rung to downgrade
// to and dies at its line.
type Ladder interface {
	Stances(rule string) []Stance
}

// NopLadder resolves no stances: the Ladder for callers with no ladder
// reports (they never downgrade), passed explicitly - nothing silently
// substitutes for a missing dependency.
type NopLadder struct{}

func (NopLadder) Stances(string) []Stance { return nil }

// Logger is the lifecycle-act logging port: Downgrades and dynamic
// deaths log at debug only (spec: Observability) - constant acts never
// flood the log.
type Logger interface {
	Log(level domain.Level, msg string, fields map[string]any)
}

// NopLogger discards every record: the Logger for callers that observe
// nothing, passed explicitly - nothing silently substitutes for a
// missing dependency.
type NopLogger struct{}

func (NopLogger) Log(domain.Level, string, map[string]any) {}

// Service is the one report lifecycle module: Trigger upserts one report per (rule, target)
// and escalates it in place - only a strictly higher ladder rung overrides a live report; every other Trigger writes nothing (ADR 0013).
type Service struct {
	repo     Repository
	trackers TrackerLookup
	ladders  Ladder
	logger   Logger

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
	updatedAt time.Time // the landed write's time: the row version a downgrade re-checks
	// inflight is non-nil only while this key's write is in flight; closed under
	// s.mu once settled (kept on success, rolled back to the prior entry on failure).
	inflight chan struct{}
}

func NewService(repo Repository, trackers TrackerLookup, ladders Ladder, logger Logger) *Service {
	return &Service{repo: repo, trackers: trackers, ladders: ladders, logger: logger, live: make(map[reportKey]*liveEntry)}
}

// alive reports whether the entry's report is alive at now: fixed
// entries by their expiry (unchanged), dynamic entries by value-based
// liveness against the tracker record of their writing key (spec:
// Trigger suppression).
func (s *Service) alive(ctx context.Context, rule string, e *liveEntry, now time.Time) bool {
	if e.dynamic {
		rec := s.trackers.Get(ctx, rule, e.key)
		return !rec.Underflow(deathLine(e.volume, e.margin), e.leak, now.Unix())
	}
	return e.expiresAt.After(now)
}

// deathLine is a report's death line: its recorded volume less its
// grace margin, the strict boundary the decayed value must stay above
// (GLOSSARY: Underflow).
func deathLine(volume, margin float64) float64 {
	return volume - margin
}

// settleEntry lands rec's shape on the entry and releases the writers
// waiting on its in-flight channel; runs under s.mu. The entry's key
// is the (ns, key) pair's key half - the writing tracker's cache key;
// ns is the rule, already named by the entry's reportKey.
func settleEntry(e *liveEntry, rec domain.Report) {
	e.dynamic = rec.Dynamic
	e.expiresAt = rec.ExpiresAt
	e.volume = rec.Tracker.Volume
	e.margin = rec.Margin
	e.leak = rec.Tracker.Leak
	_, e.key = rec.WritingKey()
	e.updatedAt = rec.UpdatedAt
	ch := e.inflight
	e.inflight = nil
	close(ch)
}

// settledRow is the row's identity as the entry settles it - the
// version a settling act re-checks against the entry the next write
// lands (its stale-row guard). Trigger knows the stored row by its
// live entry, never by a repository read.
func settledRow(key reportKey, e *liveEntry) domain.Report {
	rec := domain.Report{Rule: key.rule, Target: key.target, Dynamic: e.dynamic, UpdatedAt: e.updatedAt}
	rec.Tracker.Volume = e.volume
	return rec
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
		alive := ok && s.alive(ctx, key.rule, cur, rec.UpdatedAt)
		if alive && (!allowOverride || rec.Tracker.Volume <= cur.volume) {
			s.mu.Unlock()
			return nil
		}
		if ok && !alive && cur.dynamic {
			// The stored dynamic report is dead at write time: settle its
			// death before writing, so this write lands a fresh report at a
			// fresh created_at (spec: Dynamic report liveness) instead of an
			// upsert over the dead row freezing its identity. settleDeath
			// keeps its stale-row protections: a row that moved since this
			// decision stands and the round re-decides against it.
			s.mu.Unlock()
			if _, err := s.settleDeath(ctx, settledRow(key, cur)); err != nil {
				return fmt.Errorf("trigger report: %w", err)
			}
			continue
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
	settleEntry(e, rec)
	s.mu.Unlock()
	return nil
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
		live[reportKey{rule: rec.Rule, target: rec.Target}] = &liveEntry{expiresAt: rec.ExpiresAt, volume: rec.Tracker.Volume, updatedAt: rec.UpdatedAt}
	}
	for key, e := range s.live {
		if e.inflight != nil {
			live[key] = e
		}
	}
	s.live = live
	return nil
}

// List returns matching, alive reports (Repository.List). Lifecycle
// acts are materialized here, at document generation (spec: Dynamic
// report liveness): a dynamic report whose value has decayed to its
// death line Downgrades to the highest rung still exceeded, and dies -
// is never served, its row and live-set entry deleted with the one
// death act, so the next overflow writes a fresh report at a fresh
// created_at - when no rung remains exceeded.
func (s *Service) List(ctx context.Context, q Query) ([]domain.Report, error) {
	rows, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Report, 0, len(rows))
	for _, rec := range rows {
		if !rec.Dynamic {
			out = append(out, rec)
			continue
		}
		serve, _, err := s.materialize(ctx, rec, q.Now)
		if err != nil {
			return nil, err
		}
		if serve != nil {
			out = append(out, *serve)
		}
	}
	return out, nil
}

// decayed returns the report's writing tracker value decayed to at
// (unix seconds): the accumulation every value-bound decision reads.
func (s *Service) decayed(ctx context.Context, rec domain.Report, at int64) float64 {
	ns, key := rec.WritingKey()
	return s.trackers.Get(ctx, ns, key).ValueAt(rec.Tracker.Leak, at)
}

// downgradeTarget resolves a decaying report's restatement target: the
// highest rung strictly below the report's recorded volume whose line
// the decayed value is still strictly above - the rung's stance would
// still be alive (spec: Downgrade). ok is false when no rung remains
// exceeded: the report dies.
func downgradeTarget(stances []Stance, volume, value float64) (Stance, bool) {
	var best Stance
	found := false
	for _, st := range stances {
		if st.At >= volume || value <= deathLine(st.At, st.Margin) {
			continue
		}
		if !found || st.At > best.At {
			best, found = st, true
		}
	}
	return best, found
}

// restated returns the report restated to st at now (spec: Downgrade):
// the rung's reason, Penalty and policy move - a fixed rung's ttl runs
// from the downgrade write, a dynamic rung keeps value-bound liveness -
// and the recorded volume becomes the rung's threshold. created_at and
// the Sample (the climb event, which crossed every lower rung) stay.
func restated(rec domain.Report, st Stance, now time.Time) domain.Report {
	next := rec
	next.Reason = st.Reason
	next.Tracker.Volume = st.At
	next.UpdatedAt = now
	next.Margin = st.Margin
	next.Dynamic = st.Dynamic()
	if next.Dynamic {
		next.ExpiresAt = time.Time{}
	} else {
		next.ExpiresAt = now.Add(st.TTL)
	}
	penalty := st.Penalty
	next.Penalty = &penalty
	return next
}

// downgrade restates the report's row to st at now through the
// single-writer protocol and returns the landed restated report. It
// writes nothing and returns nil when the row moved since it was read:
// the newer stance stands (the next materialization re-decides).
func (s *Service) downgrade(ctx context.Context, rec domain.Report, st Stance, now time.Time) (*domain.Report, error) {
	key := reportKey{rule: rec.Rule, target: rec.Target}
	next := restated(rec, st, now)
	s.mu.Lock()
	cur := s.live[key]
	if cur != nil && cur.inflight != nil {
		// A write for this key is landing: this round serves the row as read.
		s.mu.Unlock()
		return nil, nil
	}
	if cur != nil && (cur.dynamic != rec.Dynamic || cur.volume != rec.Tracker.Volume || !cur.updatedAt.Equal(rec.UpdatedAt)) {
		// The row moved since it was read: the newer stance stands.
		s.mu.Unlock()
		return nil, nil
	}
	e := cur
	created := false
	if e == nil {
		e = &liveEntry{}
		s.live[key] = e
		created = true
	}
	e.inflight = make(chan struct{})
	s.mu.Unlock()

	if err := s.repo.Upsert(ctx, next); err != nil {
		s.mu.Lock()
		if s.live[key] == e {
			if created {
				delete(s.live, key)
			}
			ch := e.inflight
			e.inflight = nil
			close(ch)
		}
		s.mu.Unlock()
		return nil, fmt.Errorf("downgrade report: %w", err)
	}
	s.mu.Lock()
	settleEntry(e, next)
	s.mu.Unlock()
	s.logger.Log(domain.LevelDebug, "report downgraded", map[string]any{
		"rule": rec.Rule, "target": rec.Target, "from": rec.Reason, "to": next.Reason,
	})
	return &next, nil
}

// materialize settles one dynamic row's decay at now - the shared
// lifecycle-act shape behind document generation and the sweep (spec:
// Dynamic report liveness). The row stands while its decayed value is
// strictly above its death line; it Downgrades to the highest rung
// still exceeded; and it dies - the row deleted, the live-set entry
// dropped, one debug line - when no rung remains exceeded. serve is
// the report standing this round: the row as read while it stands or
// while its restatement was skipped, nil once dead. deleted reports
// the death's row deletion.
func (s *Service) materialize(ctx context.Context, rec domain.Report, now time.Time) (serve *domain.Report, deleted bool, err error) {
	value := s.decayed(ctx, rec, now.Unix())
	if value > deathLine(rec.Tracker.Volume, rec.Margin) {
		return &rec, false, nil
	}
	// The death line is crossed - the margin guards the downgrade line
	// too, so stances do not flap between rungs at the boundary (spec:
	// Downgrade). Downgrade while a lower rung is still exceeded; die
	// when none remains.
	st, ok := downgradeTarget(s.ladders.Stances(rec.Rule), rec.Tracker.Volume, value)
	if !ok {
		// The death materializes as an act, not a filter: the row goes,
		// so the next overflow writes a fresh report (spec: Dynamic
		// report liveness).
		deleted, err = s.settleDeath(ctx, rec)
		if err != nil {
			return nil, false, err
		}
		return nil, deleted, nil
	}
	next, err := s.downgrade(ctx, rec, st, now)
	if err != nil {
		return nil, false, err
	}
	if next == nil {
		// The row moved since it was read: the newer stance stands and
		// is re-materialized on the next round.
		return &rec, false, nil
	}
	return next, false, nil
}

// settleDeath materializes the dead report's death: its row is deleted
// and its live-set entry dropped through the single-writer protocol,
// and the act logs at debug - once, where the row goes, never once per
// document (spec: Observability). rec carries the row identity the
// death was decided against - read from the repository at
// materialization, or settled on the live entry at write time. deleted
// is false when the row moved since it was read: the newer stance
// stands and the next materialization re-decides.
func (s *Service) settleDeath(ctx context.Context, rec domain.Report) (bool, error) {
	key := reportKey{rule: rec.Rule, target: rec.Target}
	s.mu.Lock()
	cur := s.live[key]
	if cur != nil && cur.inflight != nil {
		// A write for this key is landing: the death re-decides next round.
		s.mu.Unlock()
		return false, nil
	}
	if cur != nil && (cur.dynamic != rec.Dynamic || cur.volume != rec.Tracker.Volume || !cur.updatedAt.Equal(rec.UpdatedAt)) {
		// The row moved since it was read: the newer stance stands.
		s.mu.Unlock()
		return false, nil
	}
	e := cur
	created := false
	if e == nil {
		e = &liveEntry{}
		s.live[key] = e
		created = true
	}
	e.inflight = make(chan struct{})
	s.mu.Unlock()

	n, err := s.repo.Delete(ctx, Query{Rule: rec.Rule, Target: rec.Target})
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := e.inflight
	e.inflight = nil
	close(ch)
	if err != nil {
		if s.live[key] == e && created {
			delete(s.live, key)
		}
		return false, err
	}
	if s.live[key] == e {
		delete(s.live, key)
	}
	s.logger.Log(domain.LevelDebug, "dynamic report died", map[string]any{
		"rule": rec.Rule, "target": rec.Target,
	})
	return n > 0, nil
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
// underflowed with no rung still exceeded - the lazy death materialized
// at sweep time (spec: Dynamic report liveness). A dynamic row whose
// value has decayed to its death line with a lower rung still exceeded
// Downgrades here too; only deletions count. Swept reports leave the
// live set.
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
		_, deleted, err := s.materialize(ctx, rec, now)
		if err != nil {
			return n, err
		}
		if deleted {
			n++
		}
	}
	s.mu.Lock()
	for key, e := range s.live {
		if e.inflight != nil {
			continue
		}
		if e.dynamic {
			rec := s.trackers.Get(ctx, key.rule, e.key)
			if !rec.Underflow(deathLine(e.volume, e.margin), e.leak, now.Unix()) {
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
