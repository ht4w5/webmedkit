package app

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Replay escalation ladders at the daemon seam: thresholds in download
// equivalents (already normalized by Filesize), warm-up blindness, the
// per-event cache-entry TTL from the ladder's top rung, and the same
// firing, terminality and report behaviour as rate and bandwidth
// ladders. Report rungs write with today's suppression semantics; the
// in-place override is a later change and rewrites these expectations.

// TestE2EReplayLadderRungsAreDownloadEquivalents: a replay ladder's
// thresholds mean full downloads of that path whether the file is a
// kilobyte or a hundred kilobytes - the Filesize enrichment normalizes
// the accumulation. Every exceeded rung fires lowest first with the
// rung's threshold as its reading volume.
func TestE2EReplayLadderRungsAreDownloadEquivalents(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: iso-ladder
        tracker: {track: replay, leak: 0}
        escalations:
          - at: 1
            actions: [{log: "debug,MARK:LOW"}]
          - at: 2
            actions: [{report: {reason: replay-ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// A 1000-byte file: the first 4 samples warm the size estimate and
	// are skipped; the next 3 full downloads add 1 each, so the value
	// stands above the low rung at 2 and above both rungs at 3.
	send := func(client, path string, sent int64, i int) {
		d.send(nginxLine(fmt.Sprintf("%d", 1734345934+i), client, "GET", path, 200, sent))
	}
	for i := 0; i < 4; i++ {
		send("203.0.113.7", "/small.iso", 1000, i)
	}
	d.waitProcessed(t, 4)
	if got := marks(d); len(got) != 0 {
		t.Fatalf("marks during warm-up = %v, want none (the tracker is blind until the estimate warms)", got)
	}
	for i := 4; i < 7; i++ {
		send("203.0.113.7", "/small.iso", 1000, i)
	}
	d.waitProcessed(t, 7)
	if got, want := marks(d), []string{"MARK:LOW", "MARK:LOW"}; !slices.Equal(got, want) {
		t.Fatalf("marks = %v, want %v (one low mark per event above the rung)", got, want)
	}

	// A 100000-byte file climbs the same ladder: two full downloads are
	// two download equivalents regardless of the file's size.
	for i := 7; i < 11; i++ {
		send("203.0.113.8", "/big.iso", 100000, i)
	}
	d.waitProcessed(t, 11)
	if got := marks(d); len(got) != 2 {
		t.Fatalf("marks = %v, want still 2 (the big file's warm-up is blind too)", got)
	}
	for i := 11; i < 14; i++ {
		send("203.0.113.8", "/big.iso", 100000, i)
	}
	d.waitProcessed(t, 14)
	want := []string{"MARK:LOW", "MARK:LOW", "MARK:LOW", "MARK:LOW"}
	if got := marks(d); !slices.Equal(got, want) {
		t.Fatalf("marks = %v, want %v (the same climb for both file sizes)", got, want)
	}

	// Each rung's log actions see the shared post-accumulation value
	// with the rung's own threshold as its volume.
	byTarget := map[string][]float64{}
	for _, rec := range d.logs.withMessage("MARK:LOW") {
		var reading domain.TrackerReading
		attrInto(t, rec, "tracker", &reading)
		if reading.Track != "replay" || reading.Volume != 1 {
			t.Errorf("reading = %+v, want replay with the low rung's threshold 1 as volume", reading)
		}
		target, _ := rec.Attrs["target"].(string)
		byTarget[target] = append(byTarget[target], reading.Value)
	}
	for target, values := range map[string][]float64{
		"203.0.113.7/32": {2, 3},
		"203.0.113.8/32": {2, 3},
	} {
		if !slices.Equal(byTarget[target], values) {
			t.Errorf("mark values for %s = %v, want %v", target, byTarget[target], values)
		}
	}

	// One report per (rule, Target): the top rung's write at value 3,
	// its threshold as the reading's volume.
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 2 {
		t.Fatalf("reports = %+v, want one per target", doc.Reports)
	}
	byTargetReport := map[string]reportEntry{}
	for _, r := range doc.Reports {
		byTargetReport[r.Target] = r
	}
	for target, sample := range map[string]struct {
		path string
		size int64
	}{
		"203.0.113.7/32": {path: "/small.iso", size: 1000},
		"203.0.113.8/32": {path: "/big.iso", size: 100000},
	} {
		r, ok := byTargetReport[target]
		if !ok {
			t.Fatalf("no report for %s: %+v", target, doc.Reports)
		}
		if r.Rule != "iso-ladder" || r.Reason != "replay-ban" {
			t.Errorf("report for %s = %q/%q, want iso-ladder/replay-ban", target, r.Rule, r.Reason)
		}
		if r.Tracker.Track != "replay" || r.Tracker.Value != 3 || r.Tracker.Leak != 0 || r.Tracker.Volume != 2 {
			t.Errorf("report for %s tracker = %+v, want {replay 3 0 2} (the writing rung's threshold 2 as volume)", target, r.Tracker)
		}
		if r.Sample.Path != sample.path || r.Sample.Sent != sample.size || r.Sample.Size != sample.size {
			t.Errorf("report for %s sample = %+v, want %s with size %d", target, r.Sample, sample.path, sample.size)
		}
	}
}

// TestE2EReplayLadderWarmUpSkipsTheTrackerEntirely: events whose
// Filesize is unknown skip the ladder's tracker entirely - no
// increment, no decay, no cache touch - until the estimate warms; only
// the known-size samples climb the rungs.
func TestE2EReplayLadderWarmUpSkipsTheTrackerEntirely(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: iso-ladder
        tracker: {track: replay, leak: 0}
        escalations:
          - at: 1
            actions: [{log: "debug,MARK:LOW"}]
          - at: 2
            actions: [{report: {reason: replay-ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Warm-up (the first 4 of 5 samples): the size estimate is unknown,
	// so every event is skipped entirely - nothing fires and nothing is
	// written. The /stats cache counters show the skip reaches the
	// store: one Update per event (the file-size record alone), never
	// the tracker bucket.
	for i := 0; i < 4; i++ {
		d.send(nginxLine(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000))
	}
	d.waitProcessed(t, 4)
	if got := marks(d); len(got) != 0 {
		t.Fatalf("marks during warm-up = %v, want none", got)
	}
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports during warm-up = %+v, want none", doc.Reports)
	}
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 4, "sets": 4, "misses": 1, "size": 1} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (only the file-size record, no tracker touch)", field, got, want)
		}
	}

	// Samples 5..7 are enriched (size 1000): each full transfer adds one
	// download equivalent and only those three climb - the report's
	// reading counts the known samples alone.
	for i := 4; i < 7; i++ {
		d.send(nginxLine(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000))
	}
	d.waitProcessed(t, 7)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after warm-up = %+v, want 1", doc.Reports)
	}
	if v := doc.Reports[0].Tracker.Value; v != 3 {
		t.Errorf("tracker value = %v, want 3 (the warm-up events never incremented)", v)
	}
	_, stats = d.getStats(t)
	for field, want := range map[string]float64{"misses": 2, "size": 2} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (the tracker bucket appears with the first known sample)", field, got, want)
		}
	}
}

// TestE2EReplayLadderTrackerTTLFromTopRung: the cache-entry TTL
// re-derives per event from the ladder's top rung (top/leak), so replay
// state outlives the whole climb and restarts from zero past it.
func TestE2EReplayLadderTrackerTTLFromTopRung(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: iso-ladder
        tracker: {track: replay, leak: 1}
        escalations:
          - at: 1
            actions: [{log: "debug,MARK:LOW"}]
          - at: 20
            actions: [{log: "debug,MARK:HIGH"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Warm-up: 4 of 5 samples, skipped entirely.
	for i := 0; i < 4; i++ {
		d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000))
	}
	d.waitProcessed(t, 4)

	// Events share one event second (no leak decay between them) and
	// arrive 5s apart on the wall clock - well inside the top rung's
	// 20s TTL: the bucket keeps its state and climbs 1..5, and the rung
	// at 1 fires while the value stands above it (events 2..5).
	for i := 0; i < 5; i++ {
		if i > 0 {
			clock.Advance(5 * time.Second)
		}
		d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000))
		d.waitProcessed(t, int64(5+i))
	}
	if got := marks(d); len(got) != 4 {
		t.Fatalf("marks = %v, want one per event above the rung (4)", got)
	}

	// Past the 20s TTL the entry expires and the bucket restarts empty:
	// the next event contributes 1 alone and the rung holds silent.
	clock.Advance(30 * time.Second)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000))
	d.waitProcessed(t, 10)
	if got := marks(d); len(got) != 4 {
		t.Errorf("marks = %v, want still 4 (state restarted past the TTL)", got)
	}

	// Within the TTL the climb resumes from the fresh state.
	clock.Advance(5 * time.Second)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000))
	d.waitProcessed(t, 11)
	if got := marks(d); len(got) != 5 {
		t.Errorf("marks = %v, want 5 (the climb resumes from the fresh state)", got)
	}
}

// TestE2EReplayLadderRungTerminalityAndReportBehaviour: fired rungs and
// per-fired-rung terminality behave exactly as for rate and bandwidth
// ladders - a fired jump rung runs its chain and leaves later rules
// running, a fired report rung ends the chain - and the report rung
// writes one report carrying its threshold as the reading's volume.
func TestE2EReplayLadderRungTerminalityAndReportBehaviour(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: iso-ladder
        tracker: {track: replay, leak: 0}
        escalations:
          - at: 1
            actions: [{jump: inspect}, {log: "debug,MARK:HANDOFF"}]
          - at: 2
            actions: [{report: {reason: replay-ban, ttl: 1h, penalty: drop}}]
      - id: after
        actions: [{log: "debug,MARK:AFTER"}]
  - name: inspect
    rules:
      - id: watch
        actions: [{log: "debug,MARK:WATCH"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	for i := 0; i < 7; i++ {
		d.send(nginxLine(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000))
		d.waitProcessed(t, int64(i+1))
	}

	// Warm-up events (1..4) skip the tracker: the rule does not match,
	// the later rule and the file-order inspect traversal still run.
	// Event 5 (value 1) matches nothing: no rung stands below 1 alone.
	// Event 6 (value 2) fires the handoff rung only: its jump dispatches
	// the inspect chain and the rule's chain keeps going (AFTER).
	// Event 7 (value 3) fires both rungs: the handoff runs, the report
	// rung ends the chain after the rule (no AFTER); the inspect chain's
	// own file-order traversal closes each event.
	var want []string
	for i := 0; i < 5; i++ {
		want = append(want, "MARK:AFTER", "MARK:WATCH")
	}
	want = append(want,
		"MARK:WATCH", "MARK:HANDOFF", "MARK:AFTER", "MARK:WATCH",
		"MARK:WATCH", "MARK:HANDOFF", "MARK:WATCH",
	)
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v (terminality follows the fired rungs)", got, want)
	}

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one for (iso-ladder, 203.0.113.7/32)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Rule != "iso-ladder" || r.Target != "203.0.113.7/32" || r.Reason != "replay-ban" {
		t.Errorf("report = %q/%q/%q, want iso-ladder/203.0.113.7/32/replay-ban", r.Rule, r.Target, r.Reason)
	}
	if r.Tracker.Track != "replay" || r.Tracker.Value != 3 || r.Tracker.Volume != 2 {
		t.Errorf("tracker = %+v, want {replay 3 _ 2} (the writing rung's threshold as volume)", r.Tracker)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != wallBase || r.ExpiresAt != "2025-10-17T19:00:00Z" {
		t.Errorf("created_at/updated_at/expires_at = %q/%q/%q, want a %s write with the rung's 1h ttl",
			r.CreatedAt, r.UpdatedAt, r.ExpiresAt, wallBase)
	}
	if r.Sample.Path != "/releases.iso" || r.Sample.Sent != 1000 || r.Sample.Size != 1000 {
		t.Errorf("sample = %+v, want the triggering transfer with enriched size 1000", r.Sample)
	}
	if r.Penalty == nil || r.Penalty.Action != "drop" || r.Penalty.Rate != nil {
		t.Errorf("penalty = %+v, want drop without rate", r.Penalty)
	}
}

// TestE2EReplayLadderReportRungsKeepOneReportPerRuleTarget: report rungs
// keep exactly one Report per (rule, Target) as the climb fires them,
// and `created_at` stays frozen at first flagged. (Which fired rung's
// reason, reading and timestamps the row carries follows the in-place
// override semantics landing with a later change; only the identity and
// the frozen creation time are shared by both, so only those pin here.)
func TestE2EReplayLadderReportRungsKeepOneReportPerRuleTarget(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: iso-ladder
        tracker: {track: replay, leak: 0}
        escalations:
          - at: 1
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 2
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	for i := 0; i < 4; i++ {
		d.send(nginxLine(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000))
	}
	d.waitProcessed(t, 4)

	// Samples 5 and 6 (values 1 and 2) fire the low report rung alone;
	// sample 7 (value 3) fires both report rungs. The wall clock moves
	// between the first write and the climb so a frozen created_at is
	// observable either way the row is written.
	d.send(nginxLine("1734345938", "203.0.113.7", "GET", "/releases.iso", 200, 1000))
	d.waitProcessed(t, 5)
	d.send(nginxLine("1734345939", "203.0.113.7", "GET", "/releases.iso", 200, 1000))
	d.waitProcessed(t, 6)
	clock.Advance(10 * time.Minute)
	d.send(nginxLine("1734345940", "203.0.113.7", "GET", "/releases.iso", 200, 1000))
	d.waitProcessed(t, 7)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one per (rule, Target)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Rule != "iso-ladder" || r.Target != "203.0.113.7/32" {
		t.Errorf("report = %q/%q, want iso-ladder/203.0.113.7/32", r.Rule, r.Target)
	}
	if r.CreatedAt != wallBase {
		t.Errorf("created_at = %q, want frozen %q (first flagged)", r.CreatedAt, wallBase)
	}
}
