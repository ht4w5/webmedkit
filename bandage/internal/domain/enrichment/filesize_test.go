package enrichment

import (
	"context"
	"testing"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// stubCache is a FilesizeCache test double keyed by path.
type stubCache struct {
	records map[string]FilesizeRecord
}

func (c *stubCache) Get(_ context.Context, path string) (FilesizeRecord, error) {
	return c.records[path], nil
}

func (c *stubCache) Update(_ context.Context, path string, fn func(*FilesizeRecord)) (FilesizeRecord, error) {
	if c.records == nil {
		c.records = make(map[string]FilesizeRecord)
	}
	r := c.records[path]
	fn(&r)
	c.records[path] = r
	return r, nil
}

// enrichOnce feeds one observation-eligible event (a GET 200 response
// carrying the given Content-Length) and returns its derived size.
func enrichOnce(t *testing.T, fe *FilesizeEnricher, path string, observed int64) int64 {
	t.Helper()
	return enrichEvent(t, fe, &domain.Event{Path: path, Method: "GET", Status: 200, ContentLength: observed})
}

// enrichEvent feeds one event and returns its derived size.
func enrichEvent(t *testing.T, fe *FilesizeEnricher, ev *domain.Event) int64 {
	t.Helper()
	if _, _, err := fe.Enrich(context.Background(), ev); err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	return ev.SizeBytes
}

func TestNewFilesizeEnricherValidation(t *testing.T) {
	tests := []struct {
		name    string
		period  int64
		wantErr bool
	}{
		{"valid", 5, false},
		{"period 1 is valid", 1, false},
		{"zero period rejected", 0, true},
		{"negative period rejected", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFilesizeEnricher(&stubCache{}, tt.period)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFilesizeEnricher(%d) error = %v, wantErr %v", tt.period, err, tt.wantErr)
			}
		})
	}
}

// First encounter: the run that observes a path's first response
// enriches its own event in the same run, seeded exactly - no blending,
// no waiting.
func TestEnrichSeedsFromFirstObservation(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	// The observing run's own event carries the seed exactly.
	if got := enrichOnce(t, fe, "/a", 4000); got != 4000 {
		t.Errorf("first observation's size = %d, want 4000 (seeded exactly, no blending)", got)
	}
	// Blending starts only at the second observation: 4000 + (100-4000)/5 = 3220.
	if got := enrichOnce(t, fe, "/a", 100); got != 3220 {
		t.Errorf("second observation's size = %d, want 3220", got)
	}
}

// EWMA: prev + (observed-prev)/period with int64 truncating division.
// The first row pins the seed: the first observation lands exactly.
func TestEnrichEWMAIntegerMath(t *testing.T) {
	tests := []struct {
		observed int64
		want     int64
	}{
		{1000, 1000}, // seed: first observation exactly
		{100, 820},   // 1000 + (100-1000)/5 = 1000 - 180
		{1821, 1020}, // 820 + 1001/5 = 820 + 200 (truncates .2)
		{993, 1015},  // 1020 + (-27)/5 = 1020 - 5 (truncates toward zero)
		{2000, 1212}, // 1015 + 985/5
	}
	fe, err := NewFilesizeEnricher(&stubCache{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i, tt := range tests {
		if got := enrichOnce(t, fe, "/a", tt.observed); got != tt.want {
			t.Errorf("sample %d (observed %d): size = %d, want %d", i+1, tt.observed, got, tt.want)
		}
	}
}

// period 1 means no smoothing: the estimate tracks the last observation.
func TestEnrichPeriodOneTracksObservations(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, observed := range []int64{500, 7, 3000} {
		if got := enrichOnce(t, fe, "/a", observed); got != observed {
			t.Errorf("size after %d = %d, want %d", observed, got, observed)
		}
	}
}

// SizeBytes is overwritten on every run - with the estimate when one
// exists, with FilesizeUnknown before the first observation - so a
// stale derived value never leaks through.
func TestEnrichOverwritesStaleSizeBytes(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	// Before the first observation the stale value is overwritten with unknown.
	ev := &domain.Event{Path: "/a", Method: "GET", Status: 304, SizeBytes: 999}
	if got := enrichEvent(t, fe, ev); got != domain.FilesizeUnknown {
		t.Errorf("unobserved path: size = %d, want unknown (stale value overwritten)", got)
	}
	// The first observation overwrites with the exact seed.
	ev = &domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 100, SizeBytes: 999}
	if got := enrichEvent(t, fe, ev); got != 100 {
		t.Errorf("first observation: size = %d, want 100 (stale value overwritten with the seed)", got)
	}
	// Every later run overwrites with the current estimate.
	ev = &domain.Event{Path: "/a", Method: "GET", Status: 304, SizeBytes: 999}
	if got := enrichEvent(t, fe, ev); got != 100 {
		t.Errorf("observed path: size = %d, want 100 (stale value overwritten with the estimate)", got)
	}
}

// The run-outcome table (GLOSSARY: Enrichment counter): a successful
// observation - the first or any later one - observes and enriches; a
// run without one on an observed path enriches only; any run on an
// unobserved path neither observes nor enriches.
func TestEnrichReportsRunOutcomes(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		ev           *domain.Event
		wantSize     int64
		wantObserved bool
		wantEnriched bool
	}{
		{"a run before the first observation neither observes nor enriches", &domain.Event{Path: "/a", Method: "GET", Status: 304}, domain.FilesizeUnknown, false, false},
		{"the first observation observes and enriches in the same run", &domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 1000}, 1000, true, true},
		{"a later observation observes and enriches", &domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 2000}, 1200, true, true},
		{"an ineligible run on an observed path enriches without observing", &domain.Event{Path: "/a", Method: "GET", Status: 304}, 1200, false, true},
		{"a non-GET run on an observed path enriches without observing", &domain.Event{Path: "/a", Method: "POST", Status: 200, ContentLength: 1000}, 1200, false, true},
		{"any run on an unobserved path neither observes nor enriches", &domain.Event{Path: "/b", Method: "GET", Status: 304}, domain.FilesizeUnknown, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed, enriched, err := fe.Enrich(context.Background(), tt.ev)
			if err != nil {
				t.Fatalf("Enrich error: %v", err)
			}
			if observed != tt.wantObserved || enriched != tt.wantEnriched {
				t.Errorf("outcomes = (%v, %v), want (observed %v, enriched %v)", observed, enriched, tt.wantObserved, tt.wantEnriched)
			}
			if tt.ev.SizeBytes != tt.wantSize {
				t.Errorf("size = %d, want %d", tt.ev.SizeBytes, tt.wantSize)
			}
		})
	}
}

// Estimates are per path: each path seeds from its own first
// observation, and one path's observations never move another's.
func TestEnrichStateIsPerPath(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := enrichOnce(t, fe, "/a", 1000); got != 1000 {
		t.Errorf("/a first size = %d, want 1000 (seeded from its own first observation)", got)
	}
	if got := enrichOnce(t, fe, "/b", 2000); got != 2000 {
		t.Errorf("/b first size = %d, want 2000 (seeded from its own first observation)", got)
	}
	if got := enrichOnce(t, fe, "/a", 1900); got != 1180 {
		t.Errorf("/a refined size = %d, want 1180 (1000 + 900/5, /b untouched)", got)
	}
	if got := enrichOnce(t, fe, "/b", 2900); got != 2180 {
		t.Errorf("/b refined size = %d, want 2180 (2000 + 900/5, /a untouched)", got)
	}
}

// The Sampling rule admits only GET/HEAD 200 responses carrying a
// Content-Length and GET 206 responses carrying a Content-Range total;
// every other event observes nothing and never moves the estimate.
func TestEnrichIneligibleEventsAddNoObservation(t *testing.T) {
	cache := &stubCache{}
	fe, err := NewFilesizeEnricher(cache, 5)
	if err != nil {
		t.Fatal(err)
	}
	// The first observation seeds the estimate.
	if got := enrichOnce(t, fe, "/a", 1000); got != 1000 {
		t.Fatalf("first observation: size = %d, want 1000", got)
	}
	ineligible := []*domain.Event{
		{Path: "/a", Method: "GET", Status: 304, ContentLength: 99999},     // revalidation, even carrying a length
		{Path: "/a", Method: "HEAD", Status: 200},                          // length-less HEAD
		{Path: "/a", Method: "GET", Status: 200, SentBytes: 88888},         // chunked body: transferred bytes only
		{Path: "/a", Method: "GET", Status: 206, ContentLength: 77777},     // headerless range response: a 206 never observes its Content-Length
		{Path: "/a", Method: "GET", Status: 416, ContentRangeTotal: 44444}, // 416: the authoritative total is error-path traffic
		{Path: "/a", Method: "POST", Status: 200, ContentLength: 66666},    // non-GET/HEAD
		{Path: "/a", Method: "GET", Status: 404, ContentLength: 55555},     // error body
	}
	for i, ev := range ineligible {
		if got := enrichEvent(t, fe, ev); got != 1000 {
			t.Errorf("ineligible %d (%s %d): size = %d, want the current estimate 1000", i, ev.Method, ev.Status, got)
		}
	}
	// The same ineligible events against an unobserved path leave the
	// size unknown and create no record: a record exists exactly for
	// paths with at least one observation (scanner noise leaves no
	// empty records behind to exert eviction pressure on real estimates).
	for i, tmpl := range ineligible {
		ev := *tmpl
		ev.Path = "/b"
		if got := enrichEvent(t, fe, &ev); got != domain.FilesizeUnknown {
			t.Errorf("unobserved ineligible %d (%s %d): size = %d, want unknown", i, ev.Method, ev.Status, got)
		}
	}
	if _, ok := cache.records["/b"]; ok {
		t.Error("no record created for an unobserved path")
	}
	// One more observation moves the estimate exactly one step: nothing
	// between the last observation and this one observed.
	if got := enrichOnce(t, fe, "/a", 2000); got != 1200 {
		t.Errorf("size after observation 2000 = %d, want 1200 (only observations move the estimate)", got)
	}
}

// Range responses observe the Content-Range total - the file's exact
// size - never their Content-Length; a 206 without a total observes nothing.
func TestEnrichRangeResponsesObserveContentRangeTotal(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	// The observation is the total 1000, not the Content-Length 100:
	// the first observation seeds exactly.
	if got := enrichEvent(t, fe, &domain.Event{Path: "/a", Method: "GET", Status: 206, ContentLength: 100, ContentRangeTotal: 1000}); got != 1000 {
		t.Errorf("seed from range total: size = %d, want 1000 (the total, not the Content-Length)", got)
	}
	if got := enrichEvent(t, fe, &domain.Event{Path: "/a", Method: "GET", Status: 206, ContentLength: 100, ContentRangeTotal: 500}); got != 900 {
		t.Errorf("EWMA from range total: size = %d, want 900 (1000 + (500-1000)/5)", got)
	}
	// A 206 without a usable total adds no observation and keeps the estimate.
	if got := enrichEvent(t, fe, &domain.Event{Path: "/a", Method: "GET", Status: 206, ContentLength: 77777}); got != 900 {
		t.Errorf("headerless range response: size = %d, want the current estimate 900", got)
	}
	// The next total moves the estimate exactly one step: nothing in between observed.
	if got := enrichEvent(t, fe, &domain.Event{Path: "/a", Method: "GET", Status: 206, ContentRangeTotal: 1000}); got != 920 {
		t.Errorf("size after range total 1000 = %d, want 920 (900 + 100/5)", got)
	}
}
