package app

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/infra/persistence/sqlite"
)

// Escalation ladders at the daemon seam: fired-rung dispatch and per-fired-rung
// terminality, the ladder-derived tracker TTL, and one Report per (rule, Target) whose stance escalates in place (ADR 0013).

func TestE2EEscalationFiresEveryCrossedRungLowestFirst(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 100
            actions: [{log: "debug,MARK:LOW"}]
          - at: 200
            actions: [{log: "debug,MARK:MID"}]
          - at: 300
            actions: [{log: "debug,MARK:HIGH"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 1)
	if got, want := marks(d), []string{"MARK:LOW", "MARK:MID"}; !slices.Equal(got, want) {
		t.Fatalf("marks = %v, want %v (every crossed rung, lowest first)", got, want)
	}

	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 2)
	want := []string{"MARK:LOW", "MARK:MID", "MARK:LOW", "MARK:MID", "MARK:HIGH"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v", got, want)
	}

	// Each rung's actions see the shared post-accumulation value with the rung's own threshold as its volume.
	checks := []struct {
		msg    string
		values []float64
		volume float64
	}{
		{"MARK:LOW", []float64{250, 500}, 100},
		{"MARK:MID", []float64{250, 500}, 200},
		{"MARK:HIGH", []float64{500}, 300},
	}
	for _, c := range checks {
		recs := d.logs.withMessage(c.msg)
		if len(recs) != len(c.values) {
			t.Fatalf("%s records = %d, want %d", c.msg, len(recs), len(c.values))
		}
		for i, rec := range recs {
			var reading domain.TrackerReading
			attrInto(t, rec, "tracker", &reading)
			if reading.Value != c.values[i] || reading.Volume != c.volume {
				t.Errorf("%s record %d reading = %+v, want value %v with volume %v", c.msg, i, reading, c.values[i], c.volume)
			}
		}
	}
}

// No report rungs here: nothing resets the bucket mid-climb.
func TestE2EEscalationHandoffRungFiresEveryEvent(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: gate
        tracker: {track: bandwidth, leak: 100}
        escalations:
          - at: 50
            actions: [{jump: inspect}]
          - at: 1000000
            actions: [{log: "debug,MARK:TOP"}]
  - name: inspect
    rules:
      - id: watch
        actions: [{log: "debug,MARK:WATCH"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// The inspect chain runs once per handoff: the jump dispatch is its
	// only entry, and it is silent on events the rung does not fire for.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 100))
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/a", 200, 100))
	d.send(nginxLine("1734345936.123", "203.0.113.7", "GET", "/a", 200, 100))
	d.waitProcessed(t, 3)

	d.send(nginxLine("1734345946.123", "203.0.113.7", "GET", "/a", 200, 1))
	d.waitProcessed(t, 4)

	var seq []string
	for _, rec := range d.logs.withMessage("MARK:WATCH") {
		var ev domain.Event
		attrInto(t, rec, "event", &ev)
		seq = append(seq, ev.Time.UTC().Format(time.RFC3339))
	}
	want := []string{
		eventTime1At,
		eventTime2At,
		"2024-12-16T10:45:36Z",
	}
	if !slices.Equal(seq, want) {
		t.Errorf("inspect runs per event = %v, want %v (once per handoff while the rung holds, silent after the decay)", seq, want)
	}
}

func TestE2EEscalationJumpRungNeverEndsTheChain(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{jump: topic}, {log: "debug,MARK:HANDOFF"}]
          - at: 1000
            actions: [{log: "debug,MARK:TOP"}]
      - id: after
        tracker: {track: bandwidth, leak: 0, volume: 100}
        actions: [{report: {reason: after, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
  - name: topic
    rules:
      - id: watch
        actions: [{log: "debug,MARK:TOPIC"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 1)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Rule != "after" {
		t.Fatalf("reports = %+v, want the after rule's report (a jump rung never ends the chain)", doc.Reports)
	}
	if got := marks(d); !slices.Contains(got, "MARK:HANDOFF") || !slices.Contains(got, "MARK:TOPIC") {
		t.Errorf("marks = %v, want the handoff rung's actions and the jumped chain to have run", got)
	}
}

func TestE2EEscalationGotoRungEndsChainAfterRemainingRungs(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{goto: topic}, {log: "debug,MARK:R1-TAIL"}]
          - at: 100
            actions: [{log: "debug,MARK:HIGH"}]
      - id: after
        actions: [{log: "debug,MARK:AFTER"}]
  - name: topic
    rules:
      - id: watch
        actions: [{log: "debug,MARK:TOPIC"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 1)

	// The topic chain runs at the dispatch only: the goto rung's first
	// action, before the remaining fired rungs run and the chain ends.
	want := []string{"MARK:TOPIC", "MARK:R1-TAIL", "MARK:HIGH"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v (goto dispatch, remaining fired rungs, then chain end)", got, want)
	}
}

func TestE2EEscalationReportRungEndsTheChain(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{report: {reason: first, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 1000
            actions: [{log: "debug,MARK:TOP"}]
      - id: after
        tracker: {track: bandwidth, leak: 0, volume: 100}
        actions: [{report: {reason: after, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 1)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Rule != "ladder" || doc.Reports[0].Reason != "bandage.first" {
		t.Fatalf("reports = %+v, want only the ladder rule's first report (a report rung ends the chain)", doc.Reports)
	}
}

func TestE2EEscalationTerminalityFollowsFiredRungs(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{log: "debug,MARK:LOW"}]
          - at: 1000
            actions: [{goto: topic}]
      - id: after
        actions: [{log: "debug,MARK:AFTER"}]
  - name: topic
    rules:
      - id: watch
        actions: [{log: "debug,MARK:TOPIC"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 1)

	// The goto rung never fired, so the topic chain is never dispatched:
	// nothing else enters it for the event.
	want := []string{"MARK:LOW", "MARK:AFTER"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v (terminality follows the fired rungs)", got, want)
	}
}

// The lower crossed rung's write is overridden within the same event, so the one Report stands at the top rung's stance.
func TestE2EEscalationReportRungsEscalateOneReportPerRuleTarget(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 100
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 1)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one per (rule, Target)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Rule != "ladder" || r.Reason != "bandage.ban" {
		t.Errorf("report = %q/%q, want ladder/bandage.ban (the top crossed rung's stance)", r.Rule, r.Reason)
	}
	if r.Penalty == nil || r.Penalty.Action != "drop" || r.Penalty.Rate != nil {
		t.Errorf("penalty = %+v, want the top crossed rung's drop", r.Penalty)
	}
	// The reading records the rung that wrote it: its threshold as volume.
	if r.Tracker.Value != 150 || r.Tracker.Volume != 100 {
		t.Errorf("tracker reading = %+v, want value 150 with the top crossed rung's threshold 100 as volume", r.Tracker)
	}

	clock.Advance(10 * time.Minute)
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still exactly one", doc.Reports)
	}
	r = doc.Reports[0]
	if r.UpdatedAt != wallBase || r.Tracker.Value != 150 || r.Tracker.Volume != 100 || r.Sample.Time != eventTime1At {
		t.Errorf("report = %+v, want it frozen at the first event's write (at-or-below Triggers are suppressed)", r)
	}
}

// The guard is the recorded tracker volume, rebuilt from the report table at startup.
func TestE2EEscalationOverrideGuardSurvivesRestart(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	dbPath := filepath.Join(t.TempDir(), "reports.db")
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`

	d1 := startTestDaemon(t, clock, "unixgram", dbPath, rules)
	d1.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 200))
	d1.waitProcessed(t, 1)
	doc := d1.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.ban" || doc.Reports[0].Tracker.Volume != 150 {
		t.Fatalf("reports = %+v, want one at the top crossed rung's stance", doc.Reports)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := d1.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	cancel()

	clock.Advance(10 * time.Minute) // 18:10
	d2 := startTestDaemon(t, clock, "unixgram", dbPath, rules)
	d2.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/a", 200, 60))
	d2.waitProcessed(t, 1)
	d2.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/b", 200, 100))
	d2.waitProcessed(t, 2)

	doc = d2.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.ban" || r.CreatedAt != wallBase || r.UpdatedAt != wallBase ||
		r.Tracker.Value != 200 || r.Tracker.Volume != 150 || r.Sample.Time != eventTime1At {
		t.Errorf("report = %+v, want it frozen at the pre-restart write (guard rebuilt from the recorded threshold)", r)
	}
}

// created_at alone stays frozen at first flagged; every other field takes the overriding rung's values.
func TestE2EEscalationHigherRungOverridesReportInPlace(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: squeeze, ttl: 2h, penalty: limit-req-rate, rate: 50}}]
          - at: 250
            actions: [{report: {reason: ban, ttl: 24h, penalty: reject}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 60))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.throttle" || r.Penalty.Action != "limit-bandwidth" || *r.Penalty.Rate != 1000000 {
		t.Errorf("report stance = %q %+v, want bandage.throttle/limit-bandwidth rate 1MB", r.Reason, r.Penalty)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != wallBase || r.ExpiresAt != "2025-10-17T19:00:00Z" {
		t.Errorf("created_at/updated_at/expires_at = %q/%q/%q, want the 18:00 write with the 1h ttl",
			r.CreatedAt, r.UpdatedAt, r.ExpiresAt)
	}

	clock.Advance(30 * time.Minute) // 18:30
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big2", 200, 100))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still 1 (the override moves the row in place)", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "bandage.squeeze" || r.Penalty.Action != "limit-req-rate" || *r.Penalty.Rate != 50 {
		t.Errorf("report stance = %q %+v, want bandage.squeeze/limit-req-rate rate 50 (the overriding rung's)", r.Reason, r.Penalty)
	}
	if r.Tracker.Value != 160 || r.Tracker.Volume != 150 {
		t.Errorf("tracker reading = %+v, want value 160 with the overriding rung's threshold 150", r.Tracker)
	}
	if r.Sample.Time != eventTime2At || r.Sample.Path != "/big2" || r.Sample.Sent != 100 {
		t.Errorf("sample = %+v, want the overriding event (the report's current stance is what it justifies)", r.Sample)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T18:30:00Z" || r.ExpiresAt != "2025-10-17T20:30:00Z" {
		t.Errorf("created_at/updated_at/expires_at = %q/%q/%q, want created_at frozen and the 18:30 write with the 2h ttl",
			r.CreatedAt, r.UpdatedAt, r.ExpiresAt)
	}

	clock.Advance(30 * time.Minute) // 19:00
	d.send(nginxLine("1734345936.123", "203.0.113.7", "GET", "/big3", 200, 120))
	d.waitProcessed(t, 3)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still 1", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "bandage.ban" || r.Penalty.Action != "reject" || r.Penalty.Rate != nil {
		t.Errorf("report stance = %q %+v, want bandage.ban/reject without a rate", r.Reason, r.Penalty)
	}
	if r.Tracker.Value != 280 || r.Tracker.Volume != 250 {
		t.Errorf("tracker reading = %+v, want value 280 with the overriding rung's threshold 250", r.Tracker)
	}
	if r.Sample.Time != "2024-12-16T10:45:36Z" || r.Sample.Path != "/big3" || r.Sample.Sent != 120 {
		t.Errorf("sample = %+v, want the overriding event", r.Sample)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T19:00:00Z" || r.ExpiresAt != "2025-10-18T19:00:00Z" {
		t.Errorf("created_at/updated_at/expires_at = %q/%q/%q, want created_at frozen and the 19:00 write with the 24h ttl",
			r.CreatedAt, r.UpdatedAt, r.ExpiresAt)
	}
}

// Suppression means no rewrite at all: not even a refreshed Sample or expiry.
func TestE2EEscalationLowerRungNeverDowngradesReport(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 200))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.ban" {
		t.Fatalf("reports = %+v, want one at the ban stance", doc.Reports)
	}

	clock.Advance(time.Minute)
	d.send(nginxLine("1734345994.123", "203.0.113.7", "GET", "/small", 200, 5))
	d.waitProcessed(t, 2)
	checkFrozen := func(when string) {
		t.Helper()
		doc := d.getReports(t, nil)
		if len(doc.Reports) != 1 {
			t.Fatalf("%s: reports = %+v, want 1", when, doc.Reports)
		}
		r := doc.Reports[0]
		if r.Reason != "bandage.ban" || r.UpdatedAt != wallBase || r.ExpiresAt != "2025-10-17T19:00:00Z" ||
			r.Tracker.Value != 200 || r.Tracker.Volume != 150 || r.Sample.Time != eventTime1At {
			t.Errorf("%s: report = %+v, want the first write untouched (at-or-below Triggers are suppressed)", when, r)
		}
	}
	checkFrozen("after the low rung alone fired")

	// The ban rung refires at exactly the recorded threshold 150: equal is not strictly above.
	clock.Advance(time.Minute)
	d.send(nginxLine("1734346054.123", "203.0.113.7", "GET", "/big2", 200, 100))
	d.waitProcessed(t, 3)
	checkFrozen("after an equal-threshold re-fire")
}

func TestE2EEscalationDeathRearmsAtCurrentStance(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 100
            actions: [{report: {reason: squeeze, ttl: 1h, penalty: limit-req-rate, rate: 50}}]
          - at: 150
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.ban" {
		t.Fatalf("reports = %+v, want one at the ban stance", doc.Reports)
	}
	if _, n := d.del(t, url.Values{"rule": {"ladder"}}); n != 1 {
		t.Fatalf("delete count = %d, want 1", n)
	}

	clock.Advance(110 * time.Second) // 18:01:50
	d.send(nginxLine("1734346044.123", "203.0.113.7", "GET", "/small", 200, 5))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want one fresh report", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.squeeze" || r.Penalty.Action != "limit-req-rate" || *r.Penalty.Rate != 50 {
		t.Errorf("report stance = %q %+v, want the top crossed rung's squeeze", r.Reason, r.Penalty)
	}
	if r.Tracker.Value != 145 || r.Tracker.Volume != 100 {
		t.Errorf("tracker reading = %+v, want value 145 with the top crossed rung's threshold 100", r.Tracker)
	}
	if r.CreatedAt != "2025-10-17T18:01:50Z" || r.UpdatedAt != "2025-10-17T18:01:50Z" || r.ExpiresAt != "2025-10-17T19:01:50Z" {
		t.Errorf("created_at/updated_at/expires_at = %q/%q/%q, want a fresh 18:01:50 write with the 1h ttl",
			r.CreatedAt, r.UpdatedAt, r.ExpiresAt)
	}
	if r.Sample.Time != "2024-12-16T10:47:24Z" || r.Sample.Path != "/small" {
		t.Errorf("sample = %+v, want the writing event", r.Sample)
	}
}

// failingStore fails one chosen Upsert (1-based; 0 never fails), modeling a transient write failure.
type failingStore struct {
	*sqlite.ReportRepository
	writes atomic.Int64
	failOn int64
}

func (f *failingStore) Upsert(ctx context.Context, rec domain.Report) error {
	if n := f.writes.Add(1); n == f.failOn {
		return errors.New("injected write failure")
	}
	return f.ReportRepository.Upsert(ctx, rec)
}

// The next matching event retries the override.
func TestE2EEscalationFailedOverrideKeepsTheGuard(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	inner, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store := &failingStore{ReportRepository: inner, failOn: 2}
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", rules, nil, withStore(store))

	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 200))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.throttle" {
		t.Fatalf("reports = %+v, want the landed throttle report", doc.Reports)
	}

	clock.Advance(time.Minute)
	d.send(nginxLine("1734345994.123", "203.0.113.7", "GET", "/small", 200, 5))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.throttle" || r.UpdatedAt != wallBase || r.Sample.Time != eventTime1At {
		t.Errorf("report = %+v, want the first write untouched (the failed override keeps the guard)", r)
	}

	clock.Advance(time.Minute)
	d.send(nginxLine("1734346054.123", "203.0.113.7", "GET", "/big2", 200, 100))
	d.waitProcessed(t, 3)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "bandage.ban" || r.UpdatedAt != "2025-10-17T18:02:00Z" ||
		r.Tracker.Value != 185 || r.Tracker.Volume != 150 {
		t.Errorf("report = %+v, want the retried override at 18:02 (the next matching event retries the report)", r)
	}
}

// State must outlive gaps below the top rung's TTL and never expire mid-climb.
func TestE2EEscalationTrackerTTLFromTopRung(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 1
            actions: [{log: "debug,MARK:LOW"}]
          - at: 20
            actions: [{log: "debug,MARK:HIGH"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Events share one event second (no leak decay) and sit 5s apart on the wall clock, well inside the top rung's 20s TTL.
	for i := 0; i < 5; i++ {
		if i > 0 {
			clock.Advance(5 * time.Second)
		}
		d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 1))
		d.waitProcessed(t, int64(i+1))
	}
	if got := marks(d); len(got) != 4 {
		t.Fatalf("marks = %v, want one per event above the rung (4)", got)
	}

	clock.Advance(30 * time.Second)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 1))
	d.waitProcessed(t, 6)
	if got := marks(d); len(got) != 4 {
		t.Errorf("marks = %v, want still 4 (state restarted past the TTL)", got)
	}

	clock.Advance(5 * time.Second)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 1))
	d.waitProcessed(t, 7)
	if got := marks(d); len(got) != 5 {
		t.Errorf("marks = %v, want 5 (the climb resumes from the fresh state)", got)
	}
}
