package app

import (
	"fmt"
	"strings"
	"testing"
)

// Primary seam: demand-driven enrichment at the assembled daemon -
// rules YAML and raw nginx lines in, reports, /api/stats counters and
// pointed load errors out. The enrich action runs its named enrichment
// where every action runs and nowhere else (ADR 0028): what observes
// is what the rules ask for.

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
	// consuming rule - counts the download from its very first request:
	// the first response's observation seeds the estimate and feeds the
	// tracker at once, and the third full transfer overflows (value 3 >
	// volume 2) - the same report the tracker could not write before.
	clock = newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", composedRules)
	send(d)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports with the composed enrich rule = %+v, want 1 (reports appear where the tracker was blind before)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Tracker.Value != 3 {
		t.Errorf("tracker value = %v, want 3 (three counted downloads of 1000/1000, from request one)", r.Tracker.Value)
	}
	if r.Sample.SizeBytes != 1000 {
		t.Errorf("sample size = %d, want 1000 (the enrich action fed the event before the replay rule)", r.Sample.SizeBytes)
	}
}

// TestE2EEnrichmentObservesOnlyWhereAnEnrichActionRuns: demand-driven
// learning - an eligible response contributes an observation only when
// its event reaches an enrich action, the learned estimate never leaks
// to events outside the demand domain, and untouched traffic touches no
// enrichment state at all.
func TestE2EEnrichmentObservesOnlyWhereAnEnrichActionRuns(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-source
        filter: 'src 203.0.113.7/32'
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        tracker: {track: replay, leak: 0, volume: 2}
        actions: [{report: {reason: iso-replay, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	d := startTestDaemon(t, clock, "unixgram", ":memory:", rules)

	// Eligible GETs on one path, alternating the enriched source .7
	// with the unreached source .8: only .7's responses observe and feed
	// the tracker - .7 learns the path from its first response and
	// reports at its third (value 3 > volume 2) - while .8's events
	// reach no enrich action and so observe nothing.
	for i := range 5 {
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+2*i), "203.0.113.7", "GET", "/f", 200, 1000, "1000"))
		d.send(nginxLineClen(fmt.Sprintf("%d", 1734345935+2*i), "203.0.113.8", "GET", "/f", 200, 1000, "1000"))
	}
	d.waitProcessed(t, 10)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after 5 pairs = %+v, want exactly 1 for the enriched source (the third .7 download overflows)", doc.Reports)
	}
	// The unreached source touched nothing at all: its events neither
	// observed nor fed the tracker (an unobserved path's size stays
	// unknown), so the store holds the file-size record and .7's replay
	// tracker record from 10 updates.
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 10, "sets": 10, "misses": 2, "size": 2} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (five enrich runs and five fed tracker updates)", field, got, want)
		}
	}

	// More .7 responses keep feeding the tracker (the report's reading
	// stays frozen at the first write); the interleaved .8 response
	// observes nothing and its unknown size keeps it out of the report
	// set entirely.
	for i, line := range []string{
		nginxLineClen("1734345944", "203.0.113.7", "GET", "/f", 200, 1000, "1000"),
		nginxLineClen("1734345945", "203.0.113.8", "GET", "/f", 200, 1000, "1000"),
		nginxLineClen("1734345946", "203.0.113.7", "GET", "/f", 200, 1000, "1000"),
	} {
		d.send(line)
		d.waitProcessed(t, int64(11+i))
	}
	doc = d.getReports(t, nil)
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
// the enrichment twice and observes twice, moving the estimate two EWMA
// steps per event where one enrich action moves one (pinned by the
// triggering event's sample size). Counting the runs is the Enrichment
// counters' job; preventing double enrichment is the rules author's
// (ADR 0028).
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
	send := func(d *testDaemon, sizes ...int64) {
		for i, size := range sizes {
			d.send(nginxLineClen(fmt.Sprintf("%d", 1734345934+i), "203.0.113.7", "GET", "/f", 200, size, fmt.Sprintf("%d", size)))
		}
		d.waitProcessed(t, int64(len(sizes)))
	}

	// Two enrich actions per event: observations 2 and 3 land on event
	// 2 (1000 -> 1200 -> 1360), observations 4 and 5 on event 3 (1488 ->
	// 1590). The tracker overflows on event 3 (value 3 > volume 2) and
	// the report sample carries the double-stepped estimate 1590.
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", doubleRules)
	send(d, 1000, 2000, 2000)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after 3 double-traversed events = %+v, want 1 (value 3 overflows on the third)", doc.Reports)
	}
	if v := doc.Reports[0].Tracker.Value; v != 3 {
		t.Errorf("tracker value = %v, want 3 (events 1-3 fed one download each)", v)
	}
	if got := doc.Reports[0].Sample.SizeBytes; got != 1590 {
		t.Errorf("sample size = %d, want 1590 (two observations per event: 1000->1200->1360->1488->1590)", got)
	}
	_, stats := d.getStats(t)
	for field, want := range map[string]float64{"gets": 9, "sets": 9} {
		if got := statsNum(t, stats, "tracker_cache", field); got != want {
			t.Errorf("tracker_cache.%s = %v, want %v (two enrich updates per event, one fed tracker update)", field, got, want)
		}
	}

	// One enrich action per event: the same traffic observes once per
	// event and moves the estimate one step per event (1000 -> 1200 ->
	// 1360) - the report sample exposes the difference.
	clock = newTestClock(mustTime(t, wallBase))
	single := startTestDaemon(t, clock, "unixgram", ":memory:", singleRules)
	send(single, 1000, 2000, 2000)
	doc = single.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after 3 single-traversed events = %+v, want 1", doc.Reports)
	}
	if v := doc.Reports[0].Tracker.Value; v != 3 {
		t.Errorf("tracker value = %v, want 3 (events 1-3 fed one download each)", v)
	}
	if got := doc.Reports[0].Sample.SizeBytes; got != 1360 {
		t.Errorf("sample size = %d, want 1360 (one observation per event: 1000->1200->1360)", got)
	}
}

// wantEnrichmentMigrationErr is the pinned migration-error substring: any
// presence of the old top-level enrichment block fails load naming the
// enrich actions that replaced it.
const wantEnrichmentMigrationErr = "rules: enrichment: removed - write enrich actions in your rules instead (see docs/rules.md)"

// TestE2EPointedLoadFailuresNameWhatTheyReject: every pointed load
// failure lands at the assembled-daemon seam (spec: Testing Decisions) -
// rules YAML as the input surface, the pointed load error out of the
// daemon's rules-loading path (loadTestRules - the step every started
// test daemon's rules take before assembly). A refused rules file never
// starts a daemon; its pointed error is the operator's whole outcome,
// and every error names what it rejected - a typo is a load error,
// never a silent default.
func TestE2EPointedLoadFailuresNameWhatTheyReject(t *testing.T) {
	tests := []struct {
		name    string
		rules   string
		wantErr string
	}{
		// The old top-level enrichment block is gone: any presence - even
		// empty or null - fails load with the migration error naming the
		// enrich actions that replaced it (story 14, ADR 0028).
		{name: "enrichment block fails load naming the migration", rules: "version: 0\nenrichment:\n  filter: 'src 10.0.0.0/8'\nchains:\n  - name: main\n    rules:\n      - id: r\n", wantErr: wantEnrichmentMigrationErr},
		{name: "empty enrichment block fails load naming the migration", rules: "version: 0\nenrichment: {}\nchains:\n  - name: main\n    rules:\n      - id: r\n", wantErr: wantEnrichmentMigrationErr},
		{name: "null enrichment block fails load naming the migration", rules: "version: 0\nenrichment: null\nchains:\n  - name: main\n    rules:\n      - id: r\n", wantErr: wantEnrichmentMigrationErr},
		{name: "empty enrichment key fails load naming the migration", rules: "version: 0\nenrichment:\nchains:\n  - name: main\n    rules:\n      - id: r\n", wantErr: wantEnrichmentMigrationErr},

		// An unknown enrichment type names the type - a typo can never
		// silently enrich nothing (story 12).
		{name: "enrich unknown type names the type", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: my-rule\n        actions: [{enrich: {type: fileszie}}]\n", wantErr: `rule "my-rule": action enrich: type: unknown enrichment type "fileszie" (want "filesize")`},

		// Unknown keys in the enrich block fail load naming the key
		// (story 13).
		{name: "enrich unknown key names the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, enabled: true}}]\n", wantErr: `action enrich: unknown key "enabled"`},

		// The counter key is strictly boolean when written - every
		// written non-boolean fails load naming the key (story 11,
		// GLOSSARY: Enrichment counter).
		{name: "enrich quoted counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: \"true\"}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich numeric counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: 1}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich sequence counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: [true]}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich null counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: null}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich empty counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: }}]\n", wantErr: "action enrich: counter: must be true or false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadTestRules(tt.rules)
			if err == nil {
				t.Fatalf("loadTestRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}
