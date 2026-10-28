package app

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

// Trigger tests at the daemon seam: report aliveness alone bounds the writes (one
// write per report lifetime), and nothing ever resets the tracker.

// After the report dies the next matching event writes fresh at the kept accumulation.
func TestE2EOneWritePerReportLifetime(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 0, volume: 10000}
        actions: [{report: {reason: egress-overflow, ttl: 30m, penalty: limit-bandwidth, rate: 1MB}}, {log: "debug,MARK:OVERFLOW"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 6000))
	d.waitProcessed(t, 1)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("after 1 event: %+v, want none (6000 <= 10000)", doc.Reports)
	}

	clock.Advance(10 * time.Minute) // 18:10
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 6000))
	d.waitProcessed(t, 2)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].UpdatedAt != "2025-10-17T18:10:00Z" {
		t.Fatalf("after the first overflow: %+v, want one report at 18:10", doc.Reports)
	}
	if v := doc.Reports[0].Tracker.Value; v != 12000 {
		t.Errorf("tracker value = %v, want 12000 (the Trigger-time reading)", v)
	}

	for range 2 {
		clock.Advance(10 * time.Minute)
		d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 6000))
	}
	d.waitProcessed(t, 4)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("after 4 events: %+v, want still one report", doc.Reports)
	}
	r := doc.Reports[0]
	if r.CreatedAt != "2025-10-17T18:10:00Z" || r.UpdatedAt != "2025-10-17T18:10:00Z" {
		t.Errorf("created_at/updated_at = %q/%q, want the 18:10 write frozen (suppressed)",
			r.CreatedAt, r.UpdatedAt)
	}
	if r.Tracker.Value != 12000 {
		t.Errorf("tracker value = %v, want 12000 (the first write's reading, frozen)", r.Tracker.Value)
	}
	if got := marks(d); len(got) != 3 {
		t.Errorf("log-action marks = %v, want 3 (every overflowing event matches and runs its actions)", got)
	}

	// The report dies (30m ttl) and is swept; the next matching event writes fresh.
	clock.Advance(35 * time.Minute) // 19:05 - past the 18:40 expiry
	if n, err := d.sweepOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("sweepOnce = (%d, %v), want (1, nil)", n, err)
	}
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big2", 200, 6000))
	d.waitProcessed(t, 5)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("after re-arm: %+v, want the fresh report", doc.Reports)
	}
	r = doc.Reports[0]
	if r.CreatedAt != "2025-10-17T19:05:00Z" || r.UpdatedAt != "2025-10-17T19:05:00Z" {
		t.Errorf("created_at/updated_at = %q/%q, want a fresh 19:05 write", r.CreatedAt, r.UpdatedAt)
	}
	if r.Tracker.Value != 30000 {
		t.Errorf("tracker value = %v, want 30000 (the accumulation as it stands: 4x6000 kept + 6000)", r.Tracker.Value)
	}
	if r.Sample.Path != "/big2" || r.Sample.Time != eventTime2At {
		t.Errorf("sample = %+v, want the writing event's", r.Sample)
	}
}

// singleVolumeMethodRules derives its tolerated volume per event from the step volume method.
const singleVolumeMethodRules = `
version: 0
chains:
  - name: main
    rules:
      - id: iso-replay
        tracker:
          track: replay
          leak: 0
          volume: {method: step, threshold: 1000, volume_low: 1, volume_high: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`

func warmThenOverflow(d *testDaemon, epoch int, path string, sent int64, overflow int) {
	for i := range 4 + overflow {
		d.send(nginxLine(fmt.Sprintf("%d", epoch+i), "203.0.113.7", "GET", path, 200, sent))
	}
}

// US 26 "behave exactly as before": even a later event with a larger derived volume never moves the live report - only a ladder rung may override (ADR 0013).
func TestE2ESingleVolumeRuleNeverOverridesItsLiveReport(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", singleVolumeMethodRules)

	warmThenOverflow(d, 1734345934, "/small.iso", 100, 2)
	d.waitProcessed(t, 6)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	if doc.Reports[0].Tracker.Volume != 1 {
		t.Fatalf("tracker = %+v, want the volume_low 1 estimate at the first write", doc.Reports[0].Tracker)
	}

	clock.Advance(10 * time.Minute) // 18:10
	warmThenOverflow(d, 1734345944, "/big.iso", 10000, 3)
	d.waitProcessed(t, 13)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still exactly one (one write per report lifetime)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.CreatedAt != wallBase || r.UpdatedAt != wallBase {
		t.Errorf("created_at/updated_at = %q/%q, want the 18:00 write frozen (no ladder, no override)",
			r.CreatedAt, r.UpdatedAt)
	}
	if r.Tracker.Value != 2 || r.Tracker.Volume != 1 {
		t.Errorf("tracker = %+v, want the first write's reading {value 2, volume 1} frozen", r.Tracker)
	}
	if r.Sample.Path != "/small.iso" {
		t.Errorf("sample = %+v, want the first write's /small.iso sample frozen", r.Sample)
	}
}

func TestE2ESingleVolumeRuleNeverOverridesAfterRestart(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	dbPath := filepath.Join(t.TempDir(), "reports.db")

	d1 := startTestDaemon(t, clock, "unixgram", dbPath, singleVolumeMethodRules)
	warmThenOverflow(d1, 1734345934, "/small.iso", 100, 2)
	d1.waitProcessed(t, 6)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := d1.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	cancel()

	clock.Advance(10 * time.Minute) // 18:10
	d2 := startTestDaemon(t, clock, "unixgram", dbPath, singleVolumeMethodRules)
	warmThenOverflow(d2, 1734345944, "/big.iso", 10000, 3)
	d2.waitProcessed(t, 7)
	doc := d2.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still exactly one", doc.Reports)
	}
	r := doc.Reports[0]
	if r.UpdatedAt != wallBase || r.Tracker.Value != 2 || r.Tracker.Volume != 1 || r.Sample.Path != "/small.iso" {
		t.Errorf("report = %+v, want it frozen at the pre-restart write (the bound is rebuilt from the table)", r)
	}
}

func TestE2EConcurrentTriggersLeaveTheRecordIntact(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 0, volume: 1000}
        actions: [{report: {reason: egress-overflow, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}, {log: "debug,MARK:STORM"}]
`
	d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", rules, func(c *Config) {
		c.Pipeline = Pipeline{Workers: 4}
	})

	// Timestamps are out of order on purpose; each of the 24 events
	// overflows volume 1000 on its own, so all Trigger concurrently.
	const events = 24
	times := []string{eventTime1, "1734346934.123"}
	for i := range events {
		d.send(nginxLine(times[i%2], "203.0.113.7", "GET", "/big", 200, 2000))
	}
	d.waitProcessed(t, events)

	if got := marks(d); len(got) != events {
		t.Fatalf("log-action marks = %d, want %d (every storm event matched)", len(got), events)
	}
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one", doc.Reports)
	}

	// 1000 alone could never overflow: the write's reading proves the storm's accumulation survived.
	if _, n := d.del(t, url.Values{"rule": {"egress"}}); n != 1 {
		t.Fatalf("delete count = %d, want 1", n)
	}
	clock.Advance(10 * time.Minute)
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big", 200, 1000))
	d.waitProcessed(t, events+1)

	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want the probe's fresh write", doc.Reports)
	}
	if doc.Reports[0].Tracker.Value != 49000 {
		t.Errorf("tracker value = %v, want 49000 (every concurrent event counted once; nothing reset the record)",
			doc.Reports[0].Tracker.Value)
	}
	if doc.Reports[0].UpdatedAt != "2025-10-17T18:10:00Z" {
		t.Errorf("updated_at = %q, want the 18:10 probe write", doc.Reports[0].UpdatedAt)
	}
}
