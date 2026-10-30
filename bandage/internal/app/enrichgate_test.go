package app

import (
	"fmt"
	"testing"
)

// Primary seam: the enrichment Filter gate over the assembled daemon -
// raw nginx lines in, derived fields and evaluation outcomes out.

// TestE2EEnrichmentFilterScopesTheEnrichmentPath: scoped-out traffic
// touches no enrichment state; scoped-in enriches as without the gate.
func TestE2EEnrichmentFilterScopesTheEnrichmentPath(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
enrichment:
  filter: path prefix "/files/"
chains:
  - name: main
    rules:
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Five partial downloads under /files/ warm the estimate to 1000.
	// tracker_cache counts the enrichment touch: unknown sizes skip the tracker.
	for i := 0; i < 5; i++ {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/files/a.iso", 200, 500, "1000"))
	}
	d.waitProcessed(t, 5)
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 6, "sets": 6, "misses": 2, "size": 2} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Fatalf("tracker_cache.%s = %v, want %v (warm-up)", field, got, want)
		}
	}

	// Scoped-out traffic on /other/b.iso: even eligible-shaped events touch
	// nothing - the gate is wholly out; the replay rule stays blind.
	for _, line := range []string{
		nginxLineClen("1734345939", "203.0.113.7", "GET", "/other/b.iso", 200, 500, "1000"),
		nginxLineClen("1734345940", "203.0.113.7", "GET", "/other/b.iso", 200, 500, "1000"),
		nginxLineClen("1734345941", "203.0.113.7", "GET", "/other/b.iso", 304, 0, ""),
		nginxLineClen("1734345942", "203.0.113.7", "GET", "/other/b.iso", 404, 300, "55555"),
		nginxLineClen("1734345943", "203.0.113.7", "GET", "/other/b.iso", 200, 9999, ""),
		nginxLineClen("1734345944", "203.0.113.7", "POST", "/other/b.iso", 200, 100, "100"),
	} {
		d.send(line)
	}
	d.waitProcessed(t, 11)
	_, stats = d.getStats(t)
	for field, want := range map[string]float64{"gets": 6, "sets": 6, "misses": 2, "size": 2} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (scoped-out events touch no enrichment state)", field, got, want)
		}
	}

	// Scoped-in traffic behaves exactly as without the gate: four more
	// half-downloads overflow volume 2 at 2.5 on the last.
	for i := 0; i < 4; i++ {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345945+i), "203.0.113.7", "GET", "/files/a.iso", 200, 500, "1000"))
	}
	d.waitProcessed(t, 15)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1 (the scoped-out traffic writes none)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Tracker.Value != 2.5 {
		t.Errorf("tracker value = %v, want 2.5 (the scoped-out traffic added nothing)", r.Tracker.Value)
	}
	if r.Sample.Path != "/files/a.iso" {
		t.Errorf("sample path = %q, want /files/a.iso", r.Sample.Path)
	}
	if r.Sample.Size != 1000 {
		t.Errorf("sample size = %d, want 1000", r.Sample.Size)
	}
	_, stats = d.getStats(t)
	for field, want := range map[string]float64{"gets": 14, "sets": 14, "misses": 2, "size": 2} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (nine enrichment updates, five tracker updates)", field, got, want)
		}
	}
}

// TestE2EEnrichmentFilterUnmatchedEventsCarryDefaultDerivedFields: an
// unmatched event keeps default derived fields despite a warm estimate.
func TestE2EEnrichmentFilterUnmatchedEventsCarryDefaultDerivedFields(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
enrichment:
  filter: method GET
chains:
  - name: main
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 0, volume: 100}
        actions: [{report: {reason: egress-overflow, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// GETs warm /files/a.iso to an estimate of 1000 (their own bandwidth
	// stays under volume).
	for i := 0; i < 5; i++ {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.8", "GET", "/files/a.iso", 200, 10, "1000"))
	}
	// A POST to the same path is unmatched: it overflows the bandwidth rule,
	// and its sample must carry the default 0, never the warm estimate.
	d.send(nginxLineClen("1734345939", "203.0.113.7", "POST", "/files/a.iso", 200, 120, "100"))
	d.waitProcessed(t, 6)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1 (the POST overflow)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Tracker.Value != 120 {
		t.Errorf("tracker value = %v, want 120 (evaluation ran for the unmatched event)", r.Tracker.Value)
	}
	if r.Sample.Size != 0 {
		t.Errorf("sample size = %d, want the default 0 (never the path's stale estimate)", r.Sample.Size)
	}
	// The POST touched no enrichment state: its single cache update is
	// the bandwidth tracker's.
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 11, "sets": 11, "misses": 3, "size": 3} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v", field, got, want)
		}
	}
}

// TestE2EWidenedFilterCannotPoisonTheEstimate: with enrichment.filter
// widened to any, 304/416/error/headerless traffic still never observes.
func TestE2EWidenedFilterCannotPoisonTheEstimate(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		status   int
		sent     int64
		clen     string
		wantSize int64
	}{
		{"GET 200 observes the response length", "GET", 200, 4000, "4000", 2080},
		{"304 revalidations never observe", "GET", 304, 0, "4000000", 1000},
		{"416 range errors never observe", "GET", 416, 0, "4000000", 1000},
		{"error bodies never observe", "GET", 404, 4000, "4000000", 1000},
		{"chunked 200 bodies never observe", "GET", 200, 4000, "", 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newTestClock(mustTime(t, wallBase))
			rules := `
version: 0
enrichment:
  filter: any
chains:
  - name: main
    rules:
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
			d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

			// Warm the /f estimate to 1000 (samples 1-4 stay blind).
			for i := 0; i < 5; i++ {
				d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
			}
			// Two loud events of the row's shape: admitted rows move the
			// estimate one step each (1000 -> 1600 -> 2080); the rest move nothing.
			for i := 0; i < 2; i++ {
				d.send(nginxLineClen(fmt.Sprintf("%d", 1734345939+i), "203.0.113.7", tt.method, "/f", tt.status, tt.sent, tt.clen))
			}
			// Two big chunked transfers close the row: length-less, they
			// add no observation but each counts as a full download.
			for i := 0; i < 2; i++ {
				d.send(nginxLineClen(fmt.Sprintf("%d", 1734345941+i), "203.0.113.7", "GET", "/f", 200, 10000000, ""))
			}
			d.waitProcessed(t, 9)

			doc := d.getReports(t, nil)
			if len(doc.Reports) != 1 {
				t.Fatalf("reports = %+v, want exactly 1 (the overflow)", doc.Reports)
			}
			r := doc.Reports[0]
			if r.Tracker.Value != 3 {
				t.Errorf("tracker value = %v, want 3 (three unit fractions at trigger)", r.Tracker.Value)
			}
			if r.Sample.Size != tt.wantSize {
				t.Errorf("sample size = %d, want %d", r.Sample.Size, tt.wantSize)
			}
		})
	}
}
