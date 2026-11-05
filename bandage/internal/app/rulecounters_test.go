package app

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Rule counters at the daemon seam (the spec's Testing Decisions):
// rules YAML and raw log lines in, the /api/stats rules block and the
// /metrics rule families out. The block is the primary window on a
// Rule counter (GLOSSARY: Rule counter), so every counting semantic
// pins through it.

// ruleCounterDoc mirrors one /api/stats rules block entry.
type ruleCounterDoc struct {
	ID      string
	Total   float64
	Matched float64
}

// statsRules reads the /api/stats rules block: the counter-enabled
// rules' {id, total, matched} entries in rules-file order.
func statsRules(t *testing.T, doc statsDoc) []ruleCounterDoc {
	t.Helper()
	raw, ok := doc["rules"].([]any)
	if !ok {
		t.Fatalf("/api/stats has no rules block: %v", doc)
	}
	out := make([]ruleCounterDoc, len(raw))
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("/api/stats rules[%d] = %v, want an {id, total, matched} entry", i, entry)
		}
		out[i] = ruleCounterDoc{
			ID:      m["id"].(string),
			Total:   m["total"].(float64),
			Matched: m["matched"].(float64),
		}
	}
	return out
}

// TestE2EStatsRuleCountersBlockShape: the /api/stats rules block lists
// each counter-enabled rule as {id, total, matched} in rules-file
// order across chains - counter-disabled rules stay out - with zero
// counts before any event arrives.
func TestE2EStatsRuleCountersBlockShape(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: first
        counter: true
      - id: skipped
      - id: second
        counter: true
  - name: late
    rules:
      - id: third
        counter: true
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	_, doc := d.getStats(t)
	raw, ok := doc["rules"].([]any)
	if !ok {
		t.Fatalf("/api/stats has no rules block: %v", doc)
	}
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("/api/stats rules[%d] = %v, want an object", i, entry)
		}
		if len(m) != 3 {
			t.Errorf("/api/stats rules[%d] = %v, want exactly {id, total, matched}", i, m)
		}
		for _, key := range []string{"id", "total", "matched"} {
			if _, ok := m[key]; !ok {
				t.Errorf("/api/stats rules[%d] = %v, missing %q", i, m, key)
			}
		}
	}

	want := []ruleCounterDoc{
		{ID: "first"},
		{ID: "second"},
		{ID: "third"},
	}
	got := statsRules(t, doc)
	if len(got) != len(want) {
		t.Fatalf("rules block = %+v, want %+v (rules-file order, counter-enabled only)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rules block entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// counterVisitRules is the counting-semantics fixture: one
// filter-gated rule, one unconditional rule, one counter-disabled rule.
const counterVisitRules = `
version: 0
chains:
  - name: main
    rules:
      - id: get-only
        counter: true
        filter: 'method GET'
      - id: plain
        counter: true
      - id: quiet
`

// TestE2ERuleCountersCountEveryVisitAndEveryMatch: total counts every
// event the rule is evaluated against - filter rejects included - and
// matched counts exactly the rule's Matches; both cumulative since
// daemon start.
func TestE2ERuleCountersCountEveryVisitAndEveryMatch(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", counterVisitRules)

	// Three GETs and two POSTs: every event visits both counted rules
	// (and the disabled one, silently); only GETs match get-only.
	for i := range 3 {
		d.send(nginxLine(fmt.Sprintf("173434593%d.123", i), "203.0.113.7", "GET", "/a", 200, 10))
	}
	d.send(nginxLine("1734345937.123", "203.0.113.7", "POST", "/a", 200, 10))
	d.send(nginxLine("1734345938.123", "203.0.113.7", "POST", "/a", 200, 10))
	d.waitProcessed(t, 5)

	want := []ruleCounterDoc{
		{ID: "get-only", Total: 5, Matched: 3},
		{ID: "plain", Total: 5, Matched: 5},
	}
	_, doc := d.getStats(t)
	got := statsRules(t, doc)
	if len(got) != len(want) {
		t.Fatalf("rules block = %+v, want %+v (counter-enabled rules only)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rules block entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Cumulative since start: later events add to the same counts.
	d.send(nginxLine("1734345939.123", "203.0.113.7", "GET", "/a", 200, 10))
	d.send(nginxLine("1734345940.123", "203.0.113.7", "GET", "/a", 200, 10))
	d.waitProcessed(t, 7)

	want = []ruleCounterDoc{
		{ID: "get-only", Total: 7, Matched: 5},
		{ID: "plain", Total: 7, Matched: 7},
	}
	_, doc = d.getStats(t)
	got = statsRules(t, doc)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rules block entry %d = %+v, want %+v (cumulative since start)", i, got[i], want[i])
		}
	}
}

// TestE2ERuleCountersCountOneMatchPerLadderEvent: a ladder counts one
// matched per matching Event however many rungs fired.
func TestE2ERuleCountersCountOneMatchPerLadderEvent(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        counter: true
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 100
            actions: [{log: "debug,MARK:LOW"}]
          - at: 200
            actions: [{log: "debug,MARK:MID"}]
          - at: 300
            actions: [{log: "debug,MARK:HIGH"}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Two events firing two and three rungs: five rungs run for two
	// Matches.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 250))
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/big", 200, 250))
	d.waitProcessed(t, 2)
	if got, want := marks(d), []string{"MARK:LOW", "MARK:MID", "MARK:LOW", "MARK:MID", "MARK:HIGH"}; !slices.Equal(got, want) {
		t.Fatalf("marks = %v, want %v (five rungs for the two events)", got, want)
	}

	want := []ruleCounterDoc{{ID: "ladder", Total: 2, Matched: 2}}
	_, doc := d.getStats(t)
	got := statsRules(t, doc)
	if !slices.Equal(got, want) {
		t.Errorf("rules block = %+v, want %+v (one Match per event, rungs do not count)", got, want)
	}
}

// TestE2ERuleCountersCountReplayWarmUpSkipsInTotal: a replay warm-up
// skip is a visit - the real traffic hitting the rule - but not a Match.
func TestE2ERuleCountersCountReplayWarmUpSkipsInTotal(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-size
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        counter: true
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Warm-up (the first 4 of 5 samples): the skips are visits, never
	// Matches.
	for i := range 4 {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases/24.04.iso", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 4)
	want := []ruleCounterDoc{{ID: "iso-replay", Total: 4, Matched: 0}}
	_, doc := d.getStats(t)
	if got := statsRules(t, doc); !slices.Equal(got, want) {
		t.Fatalf("rules block after warm-up = %+v, want %+v (skips are visits, not Matches)", got, want)
	}

	// The enriched samples match on the third (value 3 > volume 2).
	for i := 4; i < 7; i++ {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases/24.04.iso", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 7)
	want = []ruleCounterDoc{{ID: "iso-replay", Total: 7, Matched: 1}}
	_, doc = d.getStats(t)
	if got := statsRules(t, doc); !slices.Equal(got, want) {
		t.Errorf("rules block = %+v, want %+v", got, want)
	}
}

// TestE2ERuleCountersCountOneVisitPerDispatch: a chain dispatched more
// than once for one event counts one visit per dispatch, and each visit
// is its own Match opportunity.
func TestE2ERuleCountersCountOneVisitPerDispatch(t *testing.T) {
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
        counter: true
        actions: [{log: "debug,MARK:SHARED"}]
      - id: shared-match
        counter: true
        tracker: {track: rate, leak: 0, volume: 1}
        actions: [{report: {reason: shared, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/big", 200, 150))
	d.waitProcessed(t, 1)

	// One event, two dispatches of the shared chain: two visits each;
	// shared-seen matches on filter alone, shared-match only where its
	// tracker overflows (the second feed: value 2 > volume 1).
	want := []ruleCounterDoc{
		{ID: "shared-seen", Total: 2, Matched: 2},
		{ID: "shared-match", Total: 2, Matched: 1},
	}
	_, doc := d.getStats(t)
	got := statsRules(t, doc)
	if !slices.Equal(got, want) {
		t.Errorf("rules block = %+v, want %+v (one visit per dispatch)", got, want)
	}
}

// TestE2ERuleCountersAreExactUnderTheWorkerPool: concurrent workers do
// not lose or double counts - one visit per event and one Match per
// matching event however the workers interleave.
func TestE2ERuleCountersAreExactUnderTheWorkerPool(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", counterVisitRules, func(cfg *Config) {
		cfg.Pipeline.Workers = 4
	})

	const events = 100
	for i := range events {
		method := "GET"
		if i%2 == 1 {
			method = "POST"
		}
		d.send(nginxLine(fmt.Sprintf("%d.123", 1734345934+i), "203.0.113.7", method, "/a", 200, 10))
	}
	d.waitProcessed(t, events)

	want := []ruleCounterDoc{
		{ID: "get-only", Total: events, Matched: events / 2},
		{ID: "plain", Total: events, Matched: events},
	}
	_, doc := d.getStats(t)
	got := statsRules(t, doc)
	if !slices.Equal(got, want) {
		t.Errorf("rules block = %+v, want %+v (exact under 4 workers)", got, want)
	}
}

// orderedCounterRules is the metric-order fixture: two counter-enabled
// rules whose rules-file order (zeta, alpha) is not rule-ID order.
const orderedCounterRules = `
version: 0
chains:
  - name: main
    rules:
      - id: zeta
        counter: true
      - id: alpha
        counter: true
        filter: 'method GET'
`

// getMetrics reads the /metrics exposition body.
func getMetrics(t *testing.T, d *testDaemon) string {
	t.Helper()
	resp, err := apiClient.Get("http://" + d.apiAddr() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200 (body %q)", resp.StatusCode, body)
	}
	return string(body)
}

// ruleSample is one rule metric series: its rule label and value.
type ruleSample struct {
	Rule  string
	Value float64
}

// metricsRuleSeries reads one rule metric family's sample lines
// ("name{rule=\"<id>\"} <value>") in exposition order.
func metricsRuleSeries(t *testing.T, body, family string) []ruleSample {
	t.Helper()
	var out []ruleSample
	for _, line := range strings.Split(body, "\n") {
		rest, ok := strings.CutPrefix(line, family+`{rule="`)
		if !ok {
			continue
		}
		id, rest, ok := strings.Cut(rest, `"}`)
		if !ok {
			t.Fatalf("GET /metrics %s sample line %q: want name{rule=\"<id>\"} <value>", family, line)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			t.Fatalf("GET /metrics %s sample line %q: %v", family, line, err)
		}
		out = append(out, ruleSample{Rule: id, Value: v})
	}
	return out
}

// TestE2EMetricsRuleSeriesStartAtZeroInRuleIDOrder: the rule metric
// families emit one series per counter-enabled rule from daemon start -
// zeros before any traffic - in rule-ID order, while the /api/stats
// rules block keeps rules-file order: the two surfaces mirror fields
// and values, not ordering.
func TestE2EMetricsRuleSeriesStartAtZeroInRuleIDOrder(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", orderedCounterRules)

	body := getMetrics(t, d)
	want := []ruleSample{{Rule: "alpha", Value: 0}, {Rule: "zeta", Value: 0}}
	for _, family := range []string{"bandage_rule_events_total", "bandage_rule_events_matched_total"} {
		if got := metricsRuleSeries(t, body, family); !slices.Equal(got, want) {
			t.Errorf("%s series = %+v, want %+v (rule-ID order, zeros from daemon start)", family, got, want)
		}
	}

	_, doc := d.getStats(t)
	block := statsRules(t, doc)
	if len(block) != 2 || block[0].ID != "zeta" || block[1].ID != "alpha" {
		t.Errorf("rules block = %+v, want [zeta alpha] rules-file order beside the rule-ID-ordered series", block)
	}
}

// TestE2EMetricsRuleSeriesMirrorTheStatsRulesBlock: after traffic the
// rule metric families and the /api/stats rules block carry the same
// counts field for field - the two surfaces' mirror relationship.
func TestE2EMetricsRuleSeriesMirrorTheStatsRulesBlock(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", orderedCounterRules)

	// Two GETs and one POST: zeta matches all three, alpha (method GET)
	// is evaluated against all three and matches the two GETs.
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 10))
	d.send(nginxLine(eventTime2, "203.0.113.7", "GET", "/a", 200, 10))
	d.send(nginxLine("1734345937.123", "203.0.113.7", "POST", "/a", 200, 10))
	d.waitProcessed(t, 3)

	want := map[string]ruleCounterDoc{
		"zeta":  {ID: "zeta", Total: 3, Matched: 3},
		"alpha": {ID: "alpha", Total: 3, Matched: 2},
	}
	_, doc := d.getStats(t)
	for _, rc := range statsRules(t, doc) {
		if want[rc.ID] != rc {
			t.Errorf("rules block entry = %+v, want %+v", rc, want[rc.ID])
		}
	}
	body := getMetrics(t, d)
	for _, f := range []struct {
		family string
		field  func(rc ruleCounterDoc) float64
	}{
		{"bandage_rule_events_total", func(rc ruleCounterDoc) float64 { return rc.Total }},
		{"bandage_rule_events_matched_total", func(rc ruleCounterDoc) float64 { return rc.Matched }},
	} {
		got := metricsRuleSeries(t, body, f.family)
		if len(got) != len(want) {
			t.Fatalf("%s series = %+v, want one per counter-enabled rule", f.family, got)
		}
		for _, s := range got {
			rc, ok := want[s.Rule]
			if !ok {
				t.Errorf("%s series rule %q has no counter enabled", f.family, s.Rule)
				continue
			}
			if s.Value != f.field(rc) {
				t.Errorf("%s{rule=%q} = %v, want %v (same count the rules block carries)", f.family, s.Rule, s.Value, f.field(rc))
			}
		}
	}
}

// TestE2EStatsLogRecordKeepsItsFieldSet: the periodic stats log record
// is unchanged by decision - rule counters are a query-time surface -
// so counter-enabled rules add nothing to its field set.
func TestE2EStatsLogRecordKeepsItsFieldSet(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", orderedCounterRules)
	d.send(nginxLine(eventTime1, "203.0.113.7", "GET", "/a", 200, 10))
	d.waitProcessed(t, 1)
	d.logStats()

	recs := d.logs.withMessage("stats")
	if len(recs) != 1 {
		t.Fatalf("stats records = %d, want 1", len(recs))
	}
	want := []string{"cache_evictions", "cache_gets", "cache_misses", "cache_sets", "cache_size", "dropped", "parsed", "received"}
	got := slices.Sorted(maps.Keys(recs[0].Attrs))
	if !slices.Equal(got, want) {
		t.Errorf("stats log fields = %v, want %v (the long-standing field set, no rule counters)", got, want)
	}
}
