package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Enrichment counters at the daemon seam (the spec's Testing Decisions):
// rules YAML and raw log lines in, the /api/stats enrichment block and
// the /metrics enrichment families out. The block is the primary window
// on an Enrichment counter (GLOSSARY: Enrichment counter), so every
// counting semantic pins through it - no test reaches into the counter
// registry.

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
// value (a first or later observation does both; a run without one
// enriches from the first observation on, and neither observes nor
// enriches before it). Counts are cumulative since daemon start.
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

	// Seven runs of the one kind on one path: an ineligible POST before
	// the first observation (neither observes nor enriches), the first
	// GET observation (observe and enrich in the same run), an ineligible
	// POST on the now-observed path (enrich without observing), and four
	// later observations (observe and enrich).
	for i, method := range []string{"POST", "GET", "POST", "GET", "GET", "GET", "GET"} {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", method, "/f", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 7)

	want := []enrichCounterDoc{{Rule: "enrich-size", Enrichment: "filesize", Total: 7, Observed: 5, Enriched: 6}}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (runs and outcomes)", got, want)
	}

	// Cumulative since start: later runs add to the same row.
	d.send(nginxLineClen("1734345941.123", "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	d.send(nginxLineClen("1734345942.123", "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
	d.waitProcessed(t, 9)

	want = []enrichCounterDoc{{Rule: "enrich-size", Enrichment: "filesize", Total: 9, Observed: 7, Enriched: 8}}
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
	// enrichment twice and both runs count - each observing and
	// enriching (the first run seeds, the second refines).
	want := []enrichCounterDoc{{Rule: "shared-seen", Enrichment: "filesize", Total: 2, Observed: 2, Enriched: 2}}
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
	// count wherever the counter flag sits (observing and enriching).
	want := []enrichCounterDoc{
		{Rule: "pair-sum", Enrichment: "filesize", Total: 2, Observed: 2, Enriched: 2},
		{Rule: "pair-any", Enrichment: "filesize", Total: 2, Observed: 2, Enriched: 2},
	}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (one row per (rule, kind), every run counted)", got, want)
	}
}

// TestE2EEnrichmentCountersAreExactUnderTheWorkerPool: concurrent
// workers do not lose or double counts - one run per invocation with
// its own outcome however the workers interleave. Every eligible run
// observes and enriches from the first observation, so all three
// quantities count every run.
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

	want := []enrichCounterDoc{{Rule: "enrich-size", Enrichment: "filesize", Total: events, Observed: events, Enriched: events}}
	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); !slices.Equal(got, want) {
		t.Errorf("enrichment block = %+v, want %+v (exact under 4 workers)", got, want)
	}
}

// orderedCounterEnrichRules is the enrichment metric-order fixture: two
// counter-enabled (rule, enrichment) pairs whose rules-file order (zeta,
// alpha) is not rule-ID order.
const orderedCounterEnrichRules = `
version: 0
chains:
  - name: main
    rules:
      - id: zeta
        actions: [{enrich: {type: filesize, counter: true}}]
      - id: alpha
        filter: 'method GET'
        actions: [{enrich: {type: filesize, counter: true}}]
`

// enrichMetricSample is one enrichment metric series: its pair labels
// and value.
type enrichMetricSample struct {
	Rule       string
	Enrichment string
	Value      float64
}

// metricsEnrichmentSeries reads one enrichment metric family's sample
// lines ("name{rule=\"<id>\",enrichment=\"<kind>\"} <value>") in
// exposition order.
func metricsEnrichmentSeries(t *testing.T, body, family string) []enrichMetricSample {
	t.Helper()
	var out []enrichMetricSample
	for _, line := range strings.Split(body, "\n") {
		rest, ok := strings.CutPrefix(line, family+`{rule="`)
		if !ok {
			continue
		}
		rule, rest, ok := strings.Cut(rest, `",enrichment="`)
		if !ok {
			t.Fatalf("GET /metrics %s sample line %q: want name{rule=\"<id>\",enrichment=\"<kind>\"} <value>", family, line)
		}
		kind, rest, ok := strings.Cut(rest, `"}`)
		if !ok {
			t.Fatalf("GET /metrics %s sample line %q: want name{rule=\"<id>\",enrichment=\"<kind>\"} <value>", family, line)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			t.Fatalf("GET /metrics %s sample line %q: %v", family, line, err)
		}
		out = append(out, enrichMetricSample{Rule: rule, Enrichment: kind, Value: v})
	}
	return out
}

// TestE2EMetricsEnrichmentSeriesStartAtZeroInRulesFileOrder: the
// enrichment metric families emit one series per counter-enabled
// (rule, enrichment) pair from daemon start - zeros before any traffic
// - in rules-file order of pairs, deliberately unsorted unlike the rule
// families' rule-ID order, and rendered after the rule counter families.
func TestE2EMetricsEnrichmentSeriesStartAtZeroInRulesFileOrder(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", orderedCounterEnrichRules)

	body := getMetrics(t, d)
	want := []enrichMetricSample{
		{Rule: "zeta", Enrichment: "filesize"},
		{Rule: "alpha", Enrichment: "filesize"},
	}
	for _, family := range []string{"bandage_enrichment_runs_total", "bandage_enrichment_runs_observed_total", "bandage_enrichment_runs_enriched_total"} {
		if got := metricsEnrichmentSeries(t, body, family); !slices.Equal(got, want) {
			t.Errorf("%s series = %+v, want %+v (rules-file order of pairs, zeros from daemon start)", family, got, want)
		}
	}
	ruleFamilies := strings.Index(body, "# HELP bandage_rule_events_matched_total ")
	enrichFamilies := strings.Index(body, "# HELP bandage_enrichment_runs_total ")
	if ruleFamilies < 0 || enrichFamilies < 0 || enrichFamilies < ruleFamilies {
		t.Errorf("enrichment families do not render after the rule counter families:\n%s", body)
	}

	_, doc := d.getStats(t)
	block := statsEnrichment(t, doc)
	if len(block) != 2 || block[0].Rule != "zeta" || block[1].Rule != "alpha" {
		t.Errorf("enrichment block = %+v, want [zeta alpha] rules-file order of pairs beside the unsorted series", block)
	}
}

// TestE2EMetricsEnrichmentSeriesMirrorTheStatsEnrichmentBlock: after
// traffic the enrichment metric families and the /api/stats enrichment
// block carry the same counts field for field, in the same rules-file
// order of pairs - the two surfaces' mirror relationship, ordering
// included.
func TestE2EMetricsEnrichmentSeriesMirrorTheStatsEnrichmentBlock(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", orderedCounterEnrichRules)

	// Three eligible GETs: every event runs the enrichment at zeta and
	// at alpha (method GET) on one path - two runs per event, each
	// observing and enriching - so all three quantities move.
	d.send(nginxLineClen(eventTime1, "203.0.113.7", "GET", "/a", 200, 10, "10"))
	d.send(nginxLineClen(eventTime2, "203.0.113.7", "GET", "/a", 200, 10, "10"))
	d.send(nginxLineClen("1734345937.123", "203.0.113.7", "GET", "/a", 200, 10, "10"))
	d.waitProcessed(t, 3)

	_, doc := d.getStats(t)
	block := statsEnrichment(t, doc)
	body := getMetrics(t, d)
	for _, f := range []struct {
		family string
		field  func(ec enrichCounterDoc) float64
	}{
		{"bandage_enrichment_runs_total", func(ec enrichCounterDoc) float64 { return ec.Total }},
		{"bandage_enrichment_runs_observed_total", func(ec enrichCounterDoc) float64 { return ec.Observed }},
		{"bandage_enrichment_runs_enriched_total", func(ec enrichCounterDoc) float64 { return ec.Enriched }},
	} {
		got := metricsEnrichmentSeries(t, body, f.family)
		if len(got) != len(block) {
			t.Fatalf("%s series = %+v, want one per counter-enabled pair (as many as the enrichment block)", f.family, got)
		}
		for i, s := range got {
			ec := block[i]
			if s.Rule != ec.Rule || s.Enrichment != ec.Enrichment {
				t.Errorf("%s series %d = %s/%s, want %s/%s (same order the enrichment block lists)", f.family, i, s.Rule, s.Enrichment, ec.Rule, ec.Enrichment)
			}
			if s.Value != f.field(ec) {
				t.Errorf("%s{%s/%s} = %v, want %v (same count the enrichment block carries)", f.family, s.Rule, s.Enrichment, s.Value, f.field(ec))
			}
		}
	}
}
