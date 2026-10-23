package app

import (
	"context"
	"net/url"
	"testing"
	"time"
)

// Trigger tests at the daemon seam: report aliveness alone bounds the
// writes — one write per report lifetime, never per volume of traffic;
// a dead report re-arms a fresh write at the accumulation's current
// stance and nothing ever resets the tracker.

// TestE2EOneWritePerReportLifetime: while the report is alive further
// Triggers write nothing and the row freezes; after it dies the next
// matching event writes a fresh report whose Sample and tracker reading
// come from that event and the kept accumulation.
func TestE2EOneWritePerReportLifetime(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 0, volume: 10000}
        actions: [{report: {reason: egress-overflow, ttl: 30m, penalty: limit-bandwidth, rate: 1MB}}, {log: "debug,MARK:OVERFLOW"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// 6000 alone does not overflow volume 10000.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 6000))
	d.waitProcessed(t, 1)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("after 1 event: %+v, want none (6000 <= 10000)", doc.Reports)
	}

	// The second event overflows (12000 > 10000): the lifetime's one write.
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

	// Further overflows keep matching and Triggering, but the report is
	// alive: exactly one row per (rule, Target), updated_at does not move,
	// and the tracker record keeps accumulating (nothing resets it).
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

	// The report dies (30m after the 18:10 write) and is swept; the next
	// matching event writes a fresh report at the accumulation's current
	// stance: Sample and tracker reading from that event.
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

// TestE2EConcurrentTriggersLeaveTheRecordIntact: concurrent Triggers
// never reset the tracker — the storm's accumulation survives intact,
// every event counted once.
func TestE2EConcurrentTriggersLeaveTheRecordIntact(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: abuse
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 0, volume: 1000}
        actions: [{report: {reason: egress-overflow, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}, {log: "debug,MARK:STORM"}]
`
	d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", rules, func(c *Config) {
		c.Pipeline = Pipeline{Workers: 4}
	})

	// Timestamps are out of order on purpose; each of the 24 events
	// overflows volume 1000 on its own, so all Trigger concurrently and
	// the live report keeps their writes to one.
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

	// Delete the report and probe with exactly the volume: 1000 alone
	// could never overflow, so the write's reading proves the storm's
	// accumulation survived intact (24x2000 kept + 1000).
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
