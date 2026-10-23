package app

import (
	"slices"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Escalation ladders at the daemon seam: fired-rung dispatch,
// level-triggered handoff rungs, per-fired-rung terminality and the
// ladder-derived tracker TTL. Report rungs write with today's suppression
// semantics; the in-place override is a later change and rewrites these
// expectations.

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

// TestE2EEscalationReportRungsWriteOneReportPerRuleTarget: report rungs
// write one Report per (rule, Target) with today's suppression semantics
// - the first fired report rung writes and later Triggers stay silent
// while the report is alive. (A higher rung overriding a live report in
// place arrives with a later change and rewrites these expectations.)
func TestE2EEscalationReportRungsWriteOneReportPerRuleTarget(t *testing.T) {
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
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 2)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one per (rule, Target)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Rule != "ladder" || r.Reason != "throttle" {
		t.Errorf("report = %q/%q, want ladder/throttle (the first fired rung's write stands)", r.Rule, r.Reason)
	}
	// The reading records the rung that wrote it: its threshold as volume.
	if r.Tracker.Value != 150 || r.Tracker.Volume != 50 {
		t.Errorf("tracker reading = %+v, want value 150 with the writing rung's threshold 50 as volume", r.Tracker)
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
