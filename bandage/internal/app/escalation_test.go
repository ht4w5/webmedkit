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

// Escalation ladders at the daemon seam: fired-rung dispatch,
// level-triggered handoff rungs, per-fired-rung terminality, the
// ladder-derived tracker TTL and the report rungs' in-place escalation
// (ADR 0013): one Report per (rule, Target) whose stance follows the
// top rung the accumulation stands above.

// TestE2EEscalationFiresEveryExceededRungLowestFirst: on each matching
// event every rung the accumulated value stands above fires, lowest
// first — a transfer that flew past rungs cannot skip the lower
// responses. The fired rungs are decided once from the post-accumulation
// value, before any action runs.
func TestE2EEscalationFiresEveryExceededRungLowestFirst(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// 250 bytes stand above the low and mid rungs, not the high one.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 1)
	if got, want := marks(d), []string{"MARK:LOW", "MARK:MID"}; !slices.Equal(got, want) {
		t.Fatalf("marks = %v, want %v (every exceeded rung, lowest first)", got, want)
	}

	// The second transfer takes the value to 500: all three fire.
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 2)
	want := []string{"MARK:LOW", "MARK:MID", "MARK:LOW", "MARK:MID", "MARK:HIGH"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v", got, want)
	}

	// Each rung's actions see one reading: the shared post-accumulation
	// value with the rung's own threshold as its volume.
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

// TestE2EEscalationHandoffRungFiresEveryEvent: a handoff rung is
// level-triggered - it fires on every matching event while the value
// stays above its threshold, feeding the target chain continuously
// rather than in flickers, and stops once leak has decayed the value
// below (no report rungs here: nothing resets the bucket mid-climb).
func TestE2EEscalationHandoffRungFiresEveryEvent(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: routing
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

	// Three transfers keep the value at 100 (100 bytes added and 100
	// leaked per second), above the handoff rung: the inspect chain runs
	// twice per event (the jump dispatch plus its own file-order
	// traversal).
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 100))
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/a", 200, 100))
	d.send(nginxLine("1734345936.123", "203.0.113.7", "GET", "/a", 200, 100))
	d.waitProcessed(t, 3)

	// A quiet gap decays the value to 0: the rung stops firing and only
	// the inspect chain's own traversal runs for the next event.
	d.send(nginxLine("1734345946.123", "203.0.113.7", "GET", "/a", 200, 1))
	d.waitProcessed(t, 4)

	var seq []string
	for _, rec := range d.logs.withMessage("MARK:WATCH") {
		var ev domain.Event
		attrInto(t, rec, "event", &ev)
		seq = append(seq, ev.Time.UTC().Format(time.RFC3339))
	}
	want := []string{
		eventTime1At, eventTime1At,
		eventTime2At, eventTime2At,
		"2024-12-16T10:45:36Z", "2024-12-16T10:45:36Z",
		"2024-12-16T10:45:46Z",
	}
	if !slices.Equal(seq, want) {
		t.Errorf("inspect runs per event = %v, want %v (twice while the rung holds, once after the decay)", seq, want)
	}
}

// TestE2EEscalationJumpRungNeverEndsTheChain: a fired jump rung runs its
// target chain and resumes - the rule's later rules still see the event.
func TestE2EEscalationJumpRungNeverEndsTheChain(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

// TestE2EEscalationGotoRungEndsChainAfterRemainingRungs: a fired goto
// rung runs its target chain and ends this chain after the remaining
// fired rungs have run.
func TestE2EEscalationGotoRungEndsChainAfterRemainingRungs(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// The goto dispatch runs the topic chain, the rest of the goto rung
	// and the higher fired rung run, and the chain ends after the rule
	// (MARK:AFTER absent); the topic chain's own file-order traversal
	// closes the sequence.
	want := []string{"MARK:TOPIC", "MARK:R1-TAIL", "MARK:HIGH", "MARK:TOPIC"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v (goto dispatch, remaining fired rungs, then chain end)", got, want)
	}
}

// TestE2EEscalationReportRungEndsTheChain: a fired report rung ends the
// chain after the rule.
func TestE2EEscalationReportRungEndsTheChain(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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
	if len(doc.Reports) != 1 || doc.Reports[0].Rule != "ladder" || doc.Reports[0].Reason != "first" {
		t.Fatalf("reports = %+v, want only the ladder rule's first report (a report rung ends the chain)", doc.Reports)
	}
}

// TestE2EEscalationTerminalityFollowsFiredRungs: chain traversal ends
// only for a rung that actually fired - an unfired goto rung above the
// value leaves later rules running.
func TestE2EEscalationTerminalityFollowsFiredRungs(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// Only the low rung fired, so the chain continued (MARK:AFTER); the
	// unfired goto never dispatched its chain - the topic chain's lone
	// MARK:TOPIC is its own file-order traversal, not a dispatch.
	want := []string{"MARK:LOW", "MARK:AFTER", "MARK:TOPIC"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v (terminality follows the fired rungs)", got, want)
	}
}

// TestE2EEscalationReportRungsEscalateOneReportPerRuleTarget: report
// rungs write one Report per (rule, Target) whose stance escalates in
// place. With the value above several rungs and no live Report, one
// matching event produces one Report at the TOP crossed rung's stance:
// the lower crossed rung's write is overridden within the same event.
// Later Triggers from rungs at or below the recorded threshold are
// suppressed - no downgrade and no rewrite.
func TestE2EEscalationReportRungsEscalateOneReportPerRuleTarget(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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
	if r.Rule != "ladder" || r.Reason != "ban" {
		t.Errorf("report = %q/%q, want ladder/ban (the top crossed rung's stance)", r.Rule, r.Reason)
	}
	if r.Penalty == nil || r.Penalty.Action != "drop" || r.Penalty.Rate != nil {
		t.Errorf("penalty = %+v, want the top crossed rung's drop", r.Penalty)
	}
	// The reading records the rung that wrote it: its threshold as volume.
	if r.Tracker.Value != 150 || r.Tracker.Volume != 100 {
		t.Errorf("tracker reading = %+v, want value 150 with the top crossed rung's threshold 100 as volume", r.Tracker)
	}

	// Both rungs fire again (value 300), but their thresholds are at or
	// below the recorded one: the report stays frozen at the first
	// event's write.
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

// TestE2EEscalationOverrideGuardSurvivesRestart: the guard is the volume
// recorded in the report's tracker reading, and the live set is rebuilt
// from the report table - so a restart keeps suppressing every rung at or
// below the recorded threshold and only a strictly higher one overrides.
func TestE2EEscalationOverrideGuardSurvivesRestart(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	dbPath := filepath.Join(t.TempDir(), "reports.db")
	rules := `
version: 0
chains:
  - name: abuse
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
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "ban" || doc.Reports[0].Tracker.Volume != 150 {
		t.Fatalf("reports = %+v, want one at the top crossed rung's stance", doc.Reports)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := d1.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	cancel()

	// After the restart (tracker state starts over) the first event fires
	// only the low rung and the second fires both: every Trigger is at or
	// below the recorded threshold, so the report stays frozen at the
	// pre-restart write.
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
	if r.Reason != "ban" || r.CreatedAt != wallBase || r.UpdatedAt != wallBase ||
		r.Tracker.Value != 200 || r.Tracker.Volume != 150 || r.Sample.Time != eventTime1At {
		t.Errorf("report = %+v, want it frozen at the pre-restart write (guard rebuilt from the recorded threshold)", r)
	}
}

// TestE2EEscalationHigherRungOverridesReportInPlace: a Trigger from a
// rung strictly above the recorded threshold overrides the live report
// in place - reason, penalty, rate, Sample, tracker reading, updated_at
// and expires_at all take the overriding rung's values (expires_at runs
// from its ttl); created_at alone stays frozen at first flagged.
func TestE2EEscalationHigherRungOverridesReportInPlace(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// 60 crosses only the low rung: the report takes its stance.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 60))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "throttle" || r.Penalty.Action != "limit-bandwidth" || *r.Penalty.Rate != 1000000 {
		t.Errorf("report stance = %q %+v, want throttle/limit-bandwidth rate 1MB", r.Reason, r.Penalty)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != wallBase || r.ExpiresAt != "2025-10-17T19:00:00Z" {
		t.Errorf("created_at/updated_at/expires_at = %q/%q/%q, want the 18:00 write with the 1h ttl",
			r.CreatedAt, r.UpdatedAt, r.ExpiresAt)
	}

	// 100 more (value 160) crosses the middle rung: it overrides the
	// live report in place - everything but created_at moves.
	clock.Advance(30 * time.Minute) // 18:30
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big2", 200, 100))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still 1 (the override moves the row in place)", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "squeeze" || r.Penalty.Action != "limit-req-rate" || *r.Penalty.Rate != 50 {
		t.Errorf("report stance = %q %+v, want squeeze/limit-req-rate rate 50 (the overriding rung's)", r.Reason, r.Penalty)
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

	// 120 more (value 280) crosses the top rung: the ban moves the same
	// fields again, expires_at from its own 24h ttl.
	clock.Advance(30 * time.Minute) // 19:00
	d.send(nginxLine("1734345936.123", "203.0.113.7", "GET", "/big3", 200, 120))
	d.waitProcessed(t, 3)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still 1", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "ban" || r.Penalty.Action != "reject" || r.Penalty.Rate != nil {
		t.Errorf("report stance = %q %+v, want ban/reject without a rate", r.Reason, r.Penalty)
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

// TestE2EEscalationLowerRungNeverDowngradesReport: a Trigger from a rung
// at or below the recorded threshold is suppressed while the report is
// alive - no downgrade and no rewrite, not even a refreshed Sample or
// expiry. As leak decays the value back under the top rung, the lower
// rung's Triggers leave the report exactly as the higher rung wrote it.
func TestE2EEscalationLowerRungNeverDowngradesReport(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// 200 crosses both rungs: the report stands at the ban (threshold 150).
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 200))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "ban" {
		t.Fatalf("reports = %+v, want one at the ban stance", doc.Reports)
	}

	// 60s of leak bring the value to 145: only the low rung fires, and
	// its Trigger must not touch the live report.
	clock.Advance(time.Minute)
	d.send(nginxLine("1734345994.123", "203.0.113.7", "GET", "/small", 200, 5))
	d.waitProcessed(t, 2)
	checkFrozen := func(stage string) {
		t.Helper()
		doc := d.getReports(t, nil)
		if len(doc.Reports) != 1 {
			t.Fatalf("%s: reports = %+v, want 1", stage, doc.Reports)
		}
		r := doc.Reports[0]
		if r.Reason != "ban" || r.UpdatedAt != wallBase || r.ExpiresAt != "2025-10-17T19:00:00Z" ||
			r.Tracker.Value != 200 || r.Tracker.Volume != 150 || r.Sample.Time != eventTime1At {
			t.Errorf("%s: report = %+v, want the first write untouched (at-or-below Triggers are suppressed)", stage, r)
		}
	}
	checkFrozen("after the low rung alone fired")

	// The value climbs back to 185: both rungs fire again, but 150 is
	// exactly the recorded threshold - equal is not strictly above, so
	// nothing is rewritten either.
	clock.Advance(time.Minute)
	d.send(nginxLine("1734346054.123", "203.0.113.7", "GET", "/big2", 200, 100))
	d.waitProcessed(t, 3)
	checkFrozen("after an equal-threshold re-fire")
}

// TestE2EEscalationDeathRearmsAtCurrentStance: a dead report's stance is
// not remembered. The next matching event writes fresh at the lowest
// crossed rung and overrides up the crossed rungs within that same
// event, so the fresh report reflects the accumulation as it stands.
func TestE2EEscalationDeathRearmsAtCurrentStance(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// 250 crosses the whole ladder: the report climbs to the ban within
	// the event.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "ban" {
		t.Fatalf("reports = %+v, want one at the ban stance", doc.Reports)
	}
	if _, n := d.del(t, url.Values{"rule": {"ladder"}}); n != 1 {
		t.Fatalf("delete count = %d, want 1", n)
	}

	// 110s of leak bring the value to 145: still above several rungs, but
	// not the ban's. The next event writes at the lowest crossed rung and
	// overrides up within the same event - the fresh report stands at the
	// current stance (squeeze), not the dead report's ban.
	clock.Advance(110 * time.Second) // 18:01:50
	d.send(nginxLine("1734346044.123", "203.0.113.7", "GET", "/small", 200, 5))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want one fresh report", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "squeeze" || r.Penalty.Action != "limit-req-rate" || *r.Penalty.Rate != 50 {
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

// failingStore fails one chosen Upsert at the store seam (1-based
// sequence number; 0 never fails) to model a transient write failure.
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

// TestE2EEscalationFailedOverrideKeepsTheGuard: a failed override write
// leaves the live report standing, so the guard still holds - a lower
// rung never downgrades it - and the next matching event retries the
// override.
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
  - name: abuse
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

	// 200 crosses both rungs: the low rung's write lands and the ban's
	// override fails, leaving the throttle report as the live one.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 200))
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "throttle" {
		t.Fatalf("reports = %+v, want the landed throttle report", doc.Reports)
	}

	// The value decays to 145: only the low rung fires, and the live
	// report keeps the failed write's old stance untouched.
	clock.Advance(time.Minute)
	d.send(nginxLine("1734345994.123", "203.0.113.7", "GET", "/small", 200, 5))
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "throttle" || r.UpdatedAt != wallBase || r.Sample.Time != eventTime1At {
		t.Errorf("report = %+v, want the first write untouched (the failed override keeps the guard)", r)
	}

	// Back above the ban rung (185): the override is retried and lands.
	clock.Advance(time.Minute)
	d.send(nginxLine("1734346054.123", "203.0.113.7", "GET", "/big2", 200, 100))
	d.waitProcessed(t, 3)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "ban" || r.UpdatedAt != "2025-10-17T18:02:00Z" ||
		r.Tracker.Value != 185 || r.Tracker.Volume != 150 {
		t.Errorf("report = %+v, want the retried override at 18:02 (the next matching event retries the report)", r)
	}
}

// TestE2EEscalationTrackerTTLFromTopRung: the tracker cache-entry TTL
// derives from the ladder's top rung (top/leak) - state outlives gaps
// below it and restarts from zero past it, never expiring mid-climb.
func TestE2EEscalationTrackerTTLFromTopRung(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
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

	// Events share one event second (no leak decay between them) and
	// arrive 5s apart on the wall clock - well inside the top rung's
	// 20s TTL: the bucket keeps its state and climbs 1..5, and the rung
	// at 1 fires while the value stands above it (events 2..5).
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

	// Past the 20s TTL the entry expires and the bucket restarts empty:
	// the next event contributes 1 alone and the rung holds silent.
	clock.Advance(30 * time.Second)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 1))
	d.waitProcessed(t, 6)
	if got := marks(d); len(got) != 4 {
		t.Errorf("marks = %v, want still 4 (state restarted past the TTL)", got)
	}

	// Within the TTL the climb resumes from the fresh state.
	clock.Advance(5 * time.Second)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 1))
	d.waitProcessed(t, 7)
	if got := marks(d); len(got) != 5 {
		t.Errorf("marks = %v, want 5 (the climb resumes from the fresh state)", got)
	}
}
