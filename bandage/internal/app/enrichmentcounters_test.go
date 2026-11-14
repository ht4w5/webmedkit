package app

import (
	"strings"
	"testing"
)

// Enrichment counters at the daemon seam (the spec's Testing Decisions):
// rules YAML and raw log lines in, the /api/stats enrichment block and
// the /metrics enrichment families out. The kind vocabulary is empty -
// no enrich action loads - so no counter can move; the surfaces carry
// their explicit empty state, so a kind-free daemon's silence is never
// mistaken for a broken panel. The block's entry shape and the
// families' series contract stay pinned where they are fixture-driven
// (httpapi/metrics_test.go, httpapi/panel_test.go).

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

// TestE2EStatsEnrichmentBlockIsEmptyWithoutCounters: with no
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
        actions: [{log: "debug,SEEN:"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	_, doc := d.getStats(t)
	if got := statsEnrichment(t, doc); len(got) != 0 {
		t.Errorf("enrichment block = %+v, want [] (no counter-enabled pair)", got)
	}
}

// TestE2EMetricsEnrichmentFamiliesRenderEmptyWithoutCounters: the three
// enrichment metric families render their HELP and TYPE lines and no
// series when the rules file enables no counter - the scraper's
// explicit empty state, with the families' names and shape unchanged.
func TestE2EMetricsEnrichmentFamiliesRenderEmptyWithoutCounters(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", `
version: 0
chains:
  - name: main
    rules:
      - id: plain
        actions: [{log: "debug,SEEN:"}]
`)

	body := getMetrics(t, d)
	for _, family := range []string{"bandage_enrichment_runs_total", "bandage_enrichment_runs_observed_total", "bandage_enrichment_runs_enriched_total"} {
		if !strings.Contains(body, "# HELP "+family+" ") {
			t.Errorf("body lacks # HELP for %s", family)
		}
		if want := "# TYPE " + family + " counter\n"; !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, family) {
				t.Errorf("body renders a %s sample with no counter configured: %q", family, line)
			}
		}
	}
}
