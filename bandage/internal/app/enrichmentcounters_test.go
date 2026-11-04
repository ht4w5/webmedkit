package app

import (
	"fmt"
	"slices"
	"testing"
)

// Enrichment counters at the daemon seam (the spec's Testing Decisions):
// rules YAML and raw log lines in, the /api/stats enrichment block out.
// The block is the primary window on an Enrichment counter (GLOSSARY:
// Enrichment counter), so every counting semantic pins through it -
// no test reaches into the counter registry.

// enrichCounterDoc mirrors one /api/stats enrichment block entry.
type enrichCounterDoc struct {
	Rule       string
	Enrichment string
	Total      float64
	Observed   float64
	Enriched   float64
}

// statsEnrichment reads the /api/stats enrichment block: the
// counter-enabled (rule, enrichment) pairs' {rule, enrichment, total,
// observed, enriched} entries in rules-file order of pairs.
func statsEnrichment(t *testing.T, doc statsDoc) []enrichCounterDoc {
	t.Helper()
	raw, ok := doc["enrichment"].([]any)
	if !ok {
		t.Fatalf("/api/stats has no enrichment block: %v", doc)
	}
	out := make([]enrichCounterDoc, len(raw))
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("/api/stats enrichment[%d] = %v, want an {rule, enrichment, total, observed, enriched} entry", i, entry)
		}
		out[i] = enrichCounterDoc{
			Rule:       m["rule"].(string),
			Enrichment: m["enrichment"].(string),
			Total:      m["total"].(float64),
			Observed:   m["observed"].(float64),
			Enriched:   m["enriched"].(float64),
		}
	}
	return out
}

// TestE2EStatsEnrichmentCountersBlockShape: the /api/stats enrichment
// block lists each counter-enabled (rule, enrichment) pair as
// {rule, enrichment, total, observed, enriched} in rules-file order of
// pairs - counter-disabled pairs stay out, enrich actions in ladder
// rungs register their pair like any other - with zero counts before
// any event arrives.
func TestE2EStatsEnrichmentCountersBlockShape(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: first
        actions: [{enrich: {type: filesize, counter: true}}]
      - id: skipped
        actions: [{enrich: {type: filesize}}]
      - id: second
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 100
            actions: [{enrich: {type: filesize, counter: true}}]
          - at: 200
            actions: [stop]
  - name: late
    rules:
      - id: third
        actions: [{enrich: {type: filesize, counter: true}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	_, doc := d.getStats(t)
	raw, ok := doc["enrichment"].([]any)
	if !ok {
		t.Fatalf("/api/stats has no enrichment block: %v", doc)
	}
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("/api/stats enrichment[%d] = %v, want an object", i, entry)
		}
		if len(m) != 5 {
			t.Errorf("/api/stats enrichment[%d] = %v, want exactly {rule, enrichment, total, observed, enriched}", i, m)
		}
		for _, key := range []string{"rule", "enrichment", "total", "observed", "enriched"} {
			if _, ok := m[key]; !ok {
				t.Errorf("/api/stats enrichment[%d] = %v, missing %q", i, m, key)
			}
		}
	}

	want := []enrichCounterDoc{
		{Rule: "first", Enrichment: "filesize"},
		{Rule: "second", Enrichment: "filesize"},
		{Rule: "third", Enrichment: "filesize"},
	}
	got := statsEnrichment(t, doc)
	if len(got) != len(want) {
		t.Fatalf("enrichment block = %+v, want %+v (rules-file order of pairs, counter-enabled only)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("enrichment block entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestE2EStatsEnrichmentBlockIsEmptyWithoutCounters: without any
// counter-enabled pair the enrichment block renders as [] - present
// and empty, never null or absent.
func TestE2EStatsEnrichmentBlockIsEmptyWithoutCounters(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: plain
        actions: [{enrich: {type: filesize}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	_, doc := d.getStats(t)
	got := statsEnrichment(t, doc)
	if len(got) != 0 {
		t.Errorf("enrichment block = %+v, want [] (no counter-enabled pair)", got)
	}
}

// TestE2EEnrichmentCountersCountRunsAndOutcomes: total counts the
// enrichment's runs - not events - and observed and enriched move
// independently with the run's outcomes: observed when the run fed
// the learned state, enriched when it left a non-default derived
// value (the warm-up runs observe without enriching, a warm ineligible
// run enriches without observing). Counts are cumulative since daemon
// start.
func TestE2EEnrichmentCountersCountRunsAndOutcomes(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-size
        actions: [{enrich: {type: filesize, counter: true}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Seven runs of the one kind on one path: one ineligible POST
	// before the warm-up completes (neither observes nor enriches),
	// then the five eligible responses - the fifth leaves the warmed
	// estimate (observe and enrich) - and a warm ineligible POST that
	// enriches without observing.
	for i, method := range []string{"GET", "POST", "GET", "GET", "GET", "GET", "POST"} {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", method, "/f", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 7)

	want := []enrichCounterDoc{{Rule: "enrich-size", Enrichment: "filesize", Total: 7, Observed: 5, Enriched: 2}}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (runs and outcomes)", got, want)
	}

	// Cumulative since start: later runs add to the same row.
	d.send(nginxLineClen("1734345941.123", "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	d.send(nginxLineClen("1734345942.123", "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	d.waitProcessed(t, 9)

	want = []enrichCounterDoc{{Rule: "enrich-size", Enrichment: "filesize", Total: 9, Observed: 7, Enriched: 4}}
	_, doc = d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (cumulative since start)", got, want)
	}
}

// TestE2EEnrichmentCountersCountRunsUnderDoubleTraversal: counters
// count runs, not events - one event reaching the same enrich action
// twice through two dispatch paths counts twice (the diamond's double
// dispatch, one enrichment run per traversal).
func TestE2EEnrichmentCountersCountRunsUnderDoubleTraversal(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: via-mid
        actions: [{jump: mid}]
      - id: via-direct
        actions: [{jump: shared}]
  - name: mid
    rules:
      - id: mid-hop
        actions: [{jump: shared}]
  - name: shared
    rules:
      - id: shared-seen
        actions: [{enrich: {type: filesize, counter: true}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	d.waitProcessed(t, 1)

	// main -> mid -> shared and main -> shared: the one event runs the
	// enrichment twice and both runs count.
	want := []enrichCounterDoc{{Rule: "shared-seen", Enrichment: "filesize", Total: 2, Observed: 2}}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (one event enriched twice counts twice)", got, want)
	}
}

// TestE2EEnrichmentCountersSumOneKindPerRuleIntoOneRow: two enrich
// actions of one kind in one rule sum into one (rule, enrichment)
// row, and the pair is enabled when any of the rule's enrich actions
// of that kind carries counter: true - every run of that kind at the
// rule counts toward the row.
func TestE2EEnrichmentCountersSumOneKindPerRuleIntoOneRow(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: pair-sum
        actions: [{enrich: {type: filesize, counter: true}}, {enrich: {type: filesize}}]
      - id: pair-any
        actions: [{enrich: {type: filesize}}, {enrich: {type: filesize, counter: true}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	d.waitProcessed(t, 1)

	// One row per rule - not one per action - and both runs of the kind
	// count wherever the counter flag sits.
	want := []enrichCounterDoc{
		{Rule: "pair-sum", Enrichment: "filesize", Total: 2, Observed: 2},
		{Rule: "pair-any", Enrichment: "filesize", Total: 2, Observed: 2},
	}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (one row per (rule, kind), every run counted)", got, want)
	}
}

// TestE2EEnrichmentCountersAreExactUnderTheWorkerPool: concurrent
// workers do not lose or double counts - one run per invocation with
// its own outcome however the workers interleave. The expected
// enriched count derives from the learned state's exact sample
// positions: five runs warm the minSamples-5 estimate, so the runs
// that leave a non-default derived value are total minus the warm-up.
func TestE2EEnrichmentCountersAreExactUnderTheWorkerPool(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-size
        actions: [{enrich: {type: filesize, counter: true}}]
`
	d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", rules, func(cfg *Config) {
		cfg.Pipeline.Workers = 4
	})

	const events = 100
	for i := range events {
		d.send(nginxLineClen(fmt.Sprintf("%d.123", 1734345934+i), "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	}
	d.waitProcessed(t, events)

	want := []enrichCounterDoc{{Rule: "enrich-size", Enrichment: "filesize", Total: events, Observed: events, Enriched: events - 4}}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (exact under 4 workers)", got, want)
	}
}
