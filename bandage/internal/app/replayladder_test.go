package app

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Replay escalation ladders at the daemon seam: thresholds in download equivalents
// (normalized by Filesize), warm-up blindness, the top-rung TTL, and rate/bandwidth parity.

func TestE2EReplayLadderRungsAreDownloadEquivalents(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
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

	// The first 4 samples warm the size estimate and are skipped; only known sizes climb.
	send := func(client, path string, sent int64, i int) {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), client, "GET", path, 200, sent, fmt.Sprintf("%d", sent)))
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

	// Each rung's log actions see the shared post-accumulation value with the rung's own threshold as its volume.
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
		if r.Rule != "iso-ladder" || r.Reason != "bandage.replay-ban" {
			t.Errorf("report for %s = %q/%q, want iso-ladder/bandage.replay-ban", target, r.Rule, r.Reason)
		}
		if r.Tracker.Track != "replay" || r.Tracker.Value != 3 || r.Tracker.Leak != 0 || r.Tracker.Volume != 2 {
			t.Errorf("report for %s tracker = %+v, want {replay 3 0 2} (the writing rung's threshold 2 as volume)", target, r.Tracker)
		}
		if r.Sample.Path != sample.path || r.Sample.Sent != sample.size || r.Sample.Size != sample.size {
			t.Errorf("report for %s sample = %+v, want %s with size %d", target, r.Sample, sample.path, sample.size)
		}
	}
}

func TestE2EReplayLadderWarmUpSkipsTheTrackerEntirely(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
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

	// The /stats counters prove the skip reaches the store: one Update per event (the file-size record alone), never the tracker bucket.
	for i := 0; i < 4; i++ {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
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

	// Only the known-size samples climb: the report's reading counts those alone.
	for i := 4; i < 7; i++ {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
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

// The per-event TTL from the top rung lets replay state outlive the whole climb.
func TestE2EReplayLadderTrackerTTLFromTopRung(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
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

	for i := 0; i < 4; i++ {
		d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 4)

	// Events share one event second (no leak decay) and sit 5s apart on the wall clock, well inside the top rung's 20s TTL.
	for i := 0; i < 5; i++ {
		if i > 0 {
			clock.Advance(5 * time.Second)
		}
		d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000, "1000"))
		d.waitProcessed(t, int64(5+i))
	}
	if got := marks(d); len(got) != 4 {
		t.Fatalf("marks = %v, want one per event above the rung (4)", got)
	}

	clock.Advance(30 * time.Second)
	d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000, "1000"))
	d.waitProcessed(t, 10)
	if got := marks(d); len(got) != 4 {
		t.Errorf("marks = %v, want still 4 (state restarted past the TTL)", got)
	}

	clock.Advance(5 * time.Second)
	d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/iso", 200, 1000, "1000"))
	d.waitProcessed(t, 11)
	if got := marks(d); len(got) != 5 {
		t.Errorf("marks = %v, want 5 (the climb resumes from the fresh state)", got)
	}
}

// Parity check: fired rungs and terminality behave exactly as for rate and bandwidth ladders.
func TestE2EReplayLadderRungTerminalityAndReportBehaviour(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
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
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
		d.waitProcessed(t, int64(i+1))
	}

	// The inspect chain runs only when a rung dispatches to it: once per
	// handoff event, and not at all while the ladder is quiet.
	var want []string
	for i := 0; i < 5; i++ {
		want = append(want, "MARK:AFTER")
	}
	want = append(want,
		"MARK:WATCH", "MARK:HANDOFF", "MARK:AFTER",
		"MARK:WATCH", "MARK:HANDOFF",
	)
	if got := marks(d); !slices.Equal(got, want) {
		t.Errorf("marks = %v, want %v (terminality follows the fired rungs)", got, want)
	}

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one for (iso-ladder, 203.0.113.7/32)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Rule != "iso-ladder" || r.Target != "203.0.113.7/32" || r.Reason != "bandage.replay-ban" {
		t.Errorf("report = %q/%q/%q, want iso-ladder/203.0.113.7/32/bandage.replay-ban", r.Rule, r.Target, r.Reason)
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

// Which fired rung's fields the row carries is pinned at the override suites; only identity and frozen created_at are pinned here.
func TestE2EReplayLadderReportRungsKeepOneReportPerRuleTarget(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
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
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 4)

	// The wall clock moves between the first write and the climb so a frozen created_at is observable.
	d.send(nginxLineClen("1734345938", "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
	d.waitProcessed(t, 5)
	d.send(nginxLineClen("1734345939", "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
	d.waitProcessed(t, 6)
	clock.Advance(10 * time.Minute)
	d.send(nginxLineClen("1734345940", "203.0.113.7", "GET", "/releases.iso", 200, 1000, "1000"))
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
