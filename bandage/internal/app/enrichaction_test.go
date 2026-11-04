package app

import (
	"fmt"
	"testing"
)

// Primary seam: demand-driven enrichment at the assembled daemon -
// rules YAML and raw nginx lines in, reports and /api/stats counters
// out. The enrich action runs its named enrichment where every action
// runs and nowhere else (ADR 0028): what observes is what the rules
// ask for.

// TestE2EEnrichActionFeedsSubsequentRulesOnly: a derived field feeds
// the rules after the enrich action, never its own rule's tracker
// update - a replay rule's own enrich action can never bootstrap the
// estimate its tracker update needs, and the documented composition (a
// tracker-less enrich rule ahead of the consuming rule) turns that
// blind tracker into live volume tracking.
func TestE2EEnrichActionFeedsSubsequentRulesOnly(t *testing.T) {
	blindRules := `
version: 0
chains:
  - name: main
    rules:
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{enrich: {type: filesize}}, {report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	composedRules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-size
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	send := func(d *testDaemon) {
		for i := 0; i < 7; i++ {
			d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/releases/24.04.iso", 200, 1000, "1000"))
		}
		d.waitProcessed(t, 7)
	}

	// The enrich action beside a replay tracker never runs: the rule's
	// own tracker update is blind first, so the rule never Matches and
	// nothing observes - the estimate cannot bootstrap itself.
	clock := newTestClock(mustTime(t, wallBase))
	blind := startTestDaemon(t, clock, "unixgram", ":memory:", blindRules)
	send(blind)
	if doc := blind.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports with the self-feeding rule = %+v, want none (its own tracker update runs before its enrich action)", doc.Reports)
	}
	_, stats := blind.getStats(t)
	for _, field := range []string{"gets", "sets", "misses", "size"} {
		if got := statsNum(t, stats, "tracker_cache", field); got != 0 {
			t.Errorf("tracker_cache.%s = %v, want 0 (the starved enrich action never ran)", field, got)
		}
	}

	// The composition - a tracker-less enrich rule ahead of the same
	// consuming rule - warms the estimate and reports exactly as the
	// tracker was blind before.
	clock = newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", composedRules)
	send(d)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports with the composed enrich rule = %+v, want 1 (reports appear where the tracker was blind before)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Tracker.Value != 3 {
		t.Errorf("tracker value = %v, want 3 (three known-size downloads of 1000/1000)", r.Tracker.Value)
	}
	if r.Sample.SizeBytes != 1000 {
		t.Errorf("sample size = %d, want 1000 (the enrich action fed the event before the replay rule)", r.Sample.SizeBytes)
	}
}

// TestE2EEnrichmentObservesOnlyWhereAnEnrichActionRuns: demand-driven
// learning - an eligible response contributes an observation only when
// its event reaches an enrich action, a warm estimate never leaks to
// events outside the demand domain, and untouched traffic touches no
// enrichment state at all.
func TestE2EEnrichmentObservesOnlyWhereAnEnrichActionRuns(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-warm-source
        filter: 'src 203.0.113.7/32'
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Eligible GETs on one path, alternating the enriched source .7
	// with the unreached source .8: only .7's five responses observe,
	// so the estimate warms on the fifth .7 event - had .8's eligible
	// responses observed too, it would have warmed three pairs earlier.
	for i := range 5 {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+2*i), "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345935+2*i), "203.0.113.8", "GET", "/f", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 10)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports after 5 pairs = %+v, want none (five observations only, still warming)", doc.Reports)
	}
	// The unreached source touched nothing at all: its events neither
	// observed nor fed the tracker (warm-up skips), so the store holds
	// the file-size record and .7's replay bucket from 6 updates.
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 6, "sets": 6, "misses": 2, "size": 2} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (five enrich runs and one fed tracker update)", field, got, want)
		}
	}

	// The seventh .7 response pushes .7's bucket to 3 > 2 and reports;
	// the interleaved .8 response observes nothing and its warm-size
	// blindness keeps it out of the report set entirely.
	for i, line := range []string{
		nginxLineClen("1734345944", "203.0.113.7", "GET", "/f", 200, 1000, "1000"),
		nginxLineClen("1734345945", "203.0.113.8", "GET", "/f", 200, 1000, "1000"),
		nginxLineClen("1734345946", "203.0.113.7", "GET", "/f", 200, 1000, "1000"),
	} {
		d.send(line)
		d.waitProcessed(t, int64(11+i))
	}
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly 1 for the enriched source", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Target != "203.0.113.7/32" || r.Tracker.Value != 3 {
		t.Errorf("report = %q value %v, want 203.0.113.7/32 at value 3", r.Target, r.Tracker.Value)
	}
	if r.Sample.SizeBytes != 1000 {
		t.Errorf("sample size = %d, want 1000", r.Sample.SizeBytes)
	}
}

// TestE2EOneEventTraversingTwoEnrichActionsRunsTwice: no memoization
// per event - one event traversing two enrich actions of one kind runs
// the enrichment twice and observes twice, so three events warm a
// minSamples-5 estimate (six observations) where one enrich action
// would need five. Counting the runs is the Enrichment counters' job;
// preventing double enrichment is the rules author's (ADR 0028).
func TestE2EOneEventTraversingTwoEnrichActionsRunsTwice(t *testing.T) {
	doubleRules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-once
        actions: [{enrich: {type: filesize}}]
      - id: enrich-twice
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	singleRules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-once
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	send := func(d *testDaemon, from, to int) {
		for i := from; i < to; i++ {
			d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
		}
		d.waitProcessed(t, int64(to))
	}

	// Two enrich actions per event: observations 5 and 6 land on event
	// 3, which is fed immediately and reports on event 5 (value 3).
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", doubleRules)
	send(d, 0, 3)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports after 3 double-traversed events = %+v, want none (value 1, still under the tolerance)", doc.Reports)
	}
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 7, "sets": 7} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (two enrich updates per event, one fed tracker update)", field, got, want)
		}
	}
	send(d, 3, 5)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after 5 double-traversed events = %+v, want 1", doc.Reports)
	}
	if v := doc.Reports[0].Tracker.Value; v != 3 {
		t.Errorf("tracker value = %v, want 3 (events 3-5 fed one download each)", v)
	}

	// One enrich action per event: the same traffic observes once per
	// event and needs five events to warm - the report lands two
	// events later.
	clock = newTestClock(mustTime(t, wallBase))
	single := startTestDaemon(t, clock, "unixgram", ":memory:", singleRules)
	send(single, 0, 5)
	if doc := single.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports after 5 single-traversed events = %+v, want none (the fifth event just warmed the estimate)", doc.Reports)
	}
	send(single, 5, 7)
	doc = single.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after 7 single-traversed events = %+v, want 1", doc.Reports)
	}
	if v := doc.Reports[0].Tracker.Value; v != 3 {
		t.Errorf("tracker value = %v, want 3 (events 5-7 fed one download each)", v)
	}
}
