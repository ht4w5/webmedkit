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
	if err := fe.Enrich(context.Background(), ev); err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	return ev.SizeBytes
}

func TestNewFilesizeEnricherValidation(t *testing.T) {
	tests := []struct {
		name       string
		period     int64
		minSamples int
		wantErr    bool
	}{
		{"valid", 5, 5, false},
		{"period 1 is valid", 1, 1, false},
		{"zero period rejected", 0, 1, true},
		{"negative period rejected", -1, 1, true},
		{"zero minSamples rejected", 1, 0, true},
		{"negative minSamples rejected", 1, -3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFilesizeEnricher(&stubCache{}, tt.period, tt.minSamples)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFilesizeEnricher(%d, %d) error = %v, wantErr %v", tt.period, tt.minSamples, err, tt.wantErr)
			}
		})
	}
}

// EWMA: prev + (observed-prev)/period with int64 truncating division.
// The first row pins the seed: the first observation lands exactly,
// with no blend against an implied zero. Observations are response
// sizes (Content-Length), never transferred bytes.
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
	fe, err := NewFilesizeEnricher(&stubCache{}, 5, 1)
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
	fe, err := NewFilesizeEnricher(&stubCache{}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, observed := range []int64{500, 7, 3000} {
		if got := enrichOnce(t, fe, "/a", observed); got != observed {
			t.Errorf("size after %d = %d, want %d", observed, got, observed)
		}
	}
}

// Warm-up: SizeBytes stays unknown until minSamples, and the Nth sample's
// own event is already enriched.
func TestEnrichWarmUpGating(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	wants := []int64{0, 0, 1000, 1000, 1000}
	for i, want := range wants {
		got := enrichOnce(t, fe, "/a", 1000)
		if got != want {
			t.Errorf("sample %d: size = %d, want %d (0 = unknown until sample %d)", i+1, got, want, 3)
		}
	}
}

// While warming, SizeBytes is overwritten with FilesizeUnknown: a stale
// derived value never leaks through.
func TestEnrichOverwritesStaleSizeBytes(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	ev := &domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 100, SizeBytes: 999}
	if err := fe.Enrich(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if ev.SizeBytes != domain.FilesizeUnknown {
		t.Errorf("size = %d, want %d (stale value overwritten while warming)", ev.SizeBytes, domain.FilesizeUnknown)
	}
}

// Estimates are per path: one path's observations never move another's.
func TestEnrichStateIsPerPath(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := enrichOnce(t, fe, "/a", 1000); got != 0 {
		t.Errorf("/a first size = %d, want unknown", got)
	}
	if got := enrichOnce(t, fe, "/b", 2000); got != 0 {
		t.Errorf("/b first size = %d, want unknown", got)
	}
	if got := enrichOnce(t, fe, "/a", 1000); got != 1000 {
		t.Errorf("/a warm size = %d, want 1000", got)
	}
	if got := enrichOnce(t, fe, "/b", 2000); got != 2000 {
		t.Errorf("/b warm size = %d, want 2000", got)
	}
}

// The Sampling rule admits only GET/HEAD 200 responses carrying a
// Content-Length; every other event is enriched with the current
// estimate but adds no observation. Transferred bytes never observe,
// whatever their size.
func TestEnrichIneligibleEventsAddNoObservation(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int64{0, 0, 0, 0, 1000} {
		if got := enrichOnce(t, fe, "/a", 1000); got != want {
			t.Fatalf("warm-up sample %d: size = %d, want %d", i+1, got, want)
		}
	}
	ineligible := []*domain.Event{
		{Path: "/a", Method: "GET", Status: 304, ContentLength: 99999},  // revalidation, even carrying a length
		{Path: "/a", Method: "HEAD", Status: 200},                       // length-less HEAD
		{Path: "/a", Method: "GET", Status: 200, SentBytes: 88888},      // chunked body: transferred bytes only
		{Path: "/a", Method: "GET", Status: 206, ContentLength: 77777},  // range response
		{Path: "/a", Method: "POST", Status: 200, ContentLength: 66666}, // non-GET/HEAD
		{Path: "/a", Method: "GET", Status: 404, ContentLength: 55555},  // error body
	}
	for i, ev := range ineligible {
		if got := enrichEvent(t, fe, ev); got != 1000 {
			t.Errorf("ineligible %d (%s %d): size = %d, want the current estimate 1000", i, ev.Method, ev.Status, got)
		}
	}
	// One more observation moves the estimate exactly one step: nothing
	// between the last observation and this one observed.
	if got := enrichOnce(t, fe, "/a", 2000); got != 1200 {
		t.Errorf("size after observation 2000 = %d, want 1200 (only observations move the estimate)", got)
	}
}

// The warm-up gate covers ineligible events too: they carry the current
// estimate only from the gate on, else the unknown default.
func TestEnrichIneligibleEventsFollowWarmUpGate(t *testing.T) {
	fe, err := NewFilesizeEnricher(&stubCache{}, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	enrichOnce(t, fe, "/a", 1000)
	enrichOnce(t, fe, "/a", 1000)
	if got := enrichEvent(t, fe, &domain.Event{Path: "/a", Method: "GET", Status: 304}); got != domain.FilesizeUnknown {
		t.Errorf("ineligible during warm-up: size = %d, want unknown", got)
	}
	enrichOnce(t, fe, "/a", 1000)
	if got := enrichEvent(t, fe, &domain.Event{Path: "/a", Method: "GET", Status: 304}); got != 1000 {
		t.Errorf("ineligible after warm-up: size = %d, want the current estimate 1000", got)
	}
}
