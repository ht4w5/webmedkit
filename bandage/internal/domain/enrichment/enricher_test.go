package enrichment

import (
	"context"
	"testing"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// countingCache is a FilesizeCache double that keeps records and counts
// Update calls - the enrichment-state touch the gate must spare.
type countingCache struct {
	updates int
	records map[string]FilesizeRecord
}

func (c *countingCache) Update(_ context.Context, path string, fn func(*FilesizeRecord)) (FilesizeRecord, error) {
	c.updates++
	if c.records == nil {
		c.records = make(map[string]FilesizeRecord)
	}
	r := c.records[path]
	fn(&r)
	c.records[path] = r
	return r, nil
}

// matchFunc adapts a plain predicate to the filter seam.
type matchFunc func(*domain.Event) bool

func (f matchFunc) Match(ev *domain.Event) bool { return f(ev) }

func TestNewEnricherValidation(t *testing.T) {
	fe, err := NewFilesizeEnricher(&countingCache{}, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	yes := matchFunc(func(*domain.Event) bool { return true })
	if _, err := NewEnricher(nil, fe); err == nil {
		t.Error("NewEnricher(nil filter) succeeded, want error")
	}
	if _, err := NewEnricher(yes, nil); err == nil {
		t.Error("NewEnricher(nil enricher) succeeded, want error")
	}
}

// Unmatched events carry default derived fields and nothing else: the
// single reset clears every derived field before the gate, and no
// enrichment state is touched - zero cache updates, zero observations.
func TestEnricherUnmatchedEventsResetAndTouchNothing(t *testing.T) {
	cache := &countingCache{}
	fe, err := NewFilesizeEnricher(cache, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	en, err := NewEnricher(matchFunc(func(*domain.Event) bool { return false }), fe)
	if err != nil {
		t.Fatal(err)
	}
	events := []*domain.Event{
		// Even stale derived fields end at their defaults, before the gate.
		{Path: "/a", Method: "GET", Status: 200, ContentLength: 1000, SizeBytes: 4242},
		{Path: "/a", Method: "GET", Status: 304, SizeBytes: 999},
		{Path: "/a", Method: "POST", Status: 200, ContentLength: 1, SizeBytes: 1},
	}
	for i, ev := range events {
		if err := en.Enrich(context.Background(), ev); err != nil {
			t.Fatalf("event %d: Enrich error: %v", i, err)
		}
		if ev.SizeBytes != domain.FilesizeUnknown {
			t.Errorf("event %d: size = %d, want the default %d", i, ev.SizeBytes, domain.FilesizeUnknown)
		}
	}
	if cache.updates != 0 {
		t.Errorf("cache updates = %d, want 0 (unmatched events touch no enrichment state)", cache.updates)
	}
	if len(cache.records) != 0 {
		t.Errorf("records = %v, want none (unmatched events observe nothing)", cache.records)
	}
}

// Matched events enrich as before: eligible events observe exactly one
// response size (one cache update), matched-but-ineligible events take
// exactly one read-through update - surfacing the current estimate - and
// add no observation.
func TestEnricherMatchedEventsEnrichThroughTheGate(t *testing.T) {
	cache := &countingCache{}
	fe, err := NewFilesizeEnricher(cache, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	en, err := NewEnricher(matchFunc(func(*domain.Event) bool { return true }), fe)
	if err != nil {
		t.Fatal(err)
	}
	enrich := func(ev *domain.Event) int64 {
		t.Helper()
		if err := en.Enrich(context.Background(), ev); err != nil {
			t.Fatalf("Enrich error: %v", err)
		}
		return ev.SizeBytes
	}
	// One observation seeds exactly.
	if got := enrich(&domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 1000}); got != 1000 {
		t.Errorf("size after observation 1000 = %d, want 1000", got)
	}
	// Matched but ineligible (chunked): the current estimate, no
	// observation - the next observation still moves one EWMA step.
	if got := enrich(&domain.Event{Path: "/a", Method: "GET", Status: 200, SentBytes: 99999}); got != 1000 {
		t.Errorf("size after ineligible event = %d, want the current estimate 1000", got)
	}
	if got := enrich(&domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 2000}); got != 1200 {
		t.Errorf("size after observation 2000 = %d, want 1200 (one step; the ineligible event observed nothing)", got)
	}
	// One cache update per event: the ineligible event's is read-through.
	if want := 3; cache.updates != want {
		t.Errorf("cache updates = %d, want %d (one per matched event)", cache.updates, want)
	}
}

// The Sampling rule outranks the Filter: a widened filter (everything
// matches) still admits no 304, 416, error or headerless traffic into
// the estimate - only eligible observations move the size sequence.
func TestEnricherSamplingRuleOutranksTheFilter(t *testing.T) {
	cache := &countingCache{}
	fe, err := NewFilesizeEnricher(cache, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	en, err := NewEnricher(matchFunc(func(*domain.Event) bool { return true }), fe)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ev   *domain.Event
		want int64
	}{
		{"eligible observes the response size", &domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 1000}, 1000},
		{"304 never observes", &domain.Event{Path: "/a", Method: "GET", Status: 304, ContentLength: 9999999}, 1000},
		{"416 never observes", &domain.Event{Path: "/a", Method: "GET", Status: 416, ContentLength: 9999999}, 1000},
		{"error bodies never observe", &domain.Event{Path: "/a", Method: "GET", Status: 404, ContentLength: 9999999}, 1000},
		{"headerless never observes", &domain.Event{Path: "/a", Method: "GET", Status: 200, SentBytes: 9999999}, 1000},
		{"length-less HEAD never observes", &domain.Event{Path: "/a", Method: "HEAD", Status: 200}, 1000},
		{"eligible observes again", &domain.Event{Path: "/a", Method: "GET", Status: 200, ContentLength: 2000}, 1200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := en.Enrich(context.Background(), tt.ev); err != nil {
				t.Fatalf("Enrich error: %v", err)
			}
			if tt.ev.SizeBytes != tt.want {
				t.Errorf("size = %d, want %d", tt.ev.SizeBytes, tt.want)
			}
		})
	}
}
