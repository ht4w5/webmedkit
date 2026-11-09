package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/HT4w5/webmedkit/bandage/internal/infra/persistence/sqlite"
)

// Dynamic report death at the daemon seam (spec: Dynamic report
// liveness, Trigger suppression): a report action without ttl writes a
// report alive while its tracker value is strictly above its recorded
// volume, dying at Underflow - lazily, at document generation and
// sweep time. Event times sit on the injected wall clock's timeline,
// so clock advance is the decay driver.

// dynamicRules is the single-volume dynamic form: no ttl on the report
// action, so its life is bound to the tracker value.
const dynamicRules = `
version: 0
chains:
  - name: main
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 1000, volume: 10000}
        actions:
          - report: {reason: egress-overflow, penalty: limit-bandwidth, rate: 1MB}
          - log: "debug,MARK:OVERFLOW"
`

// dynamicLadderRules mixes rungs on one dynamic ladder: both report
// actions carry no ttl.
const dynamicLadderRules = `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, penalty: drop}}]
`

// sendAt sends one 6000-byte line at the wall-aligned unix second t.
func sendAt(d *testDaemon, t, client, path string, sent int64) {
	d.send(nginxLine(t, client, "GET", path, 200, sent))
}

// TestE2EDynamicReportServesWithNullExpiry: a ttl-less report action on
// a tracked rule writes a dynamic report, served with "dynamic": true
// after "penalty" and "expires_at": null.
func TestE2EDynamicReportServesWithNullExpiry(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicRules)

	// 6000 + 6000 - 1000*1s = 11000 > 10000: the second event overflows.
	sendAt(d, "1760724000", "203.0.113.7", "/releases/24.04.iso", 6000)
	sendAt(d, "1760724001", "203.0.113.7", "/releases/24.04.iso", 6000)
	d.waitProcessed(t, 2)
	clock.Advance(time.Second) // 18:00:01

	body := d.getReportsRaw(t)
	doc := d.getReports(t, nil)
	if doc.Version != 0 {
		t.Errorf("document version = %d, want 0", doc.Version)
	}
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if !r.Dynamic {
		t.Errorf("dynamic = false, want true (a ttl-less report action)")
	}
	if r.ExpiresAt != "" {
		t.Errorf("expires_at = %q, want null (decoded empty)", r.ExpiresAt)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != wallBase {
		t.Errorf("created_at/updated_at = %q/%q, want the %q write", r.CreatedAt, r.UpdatedAt, wallBase)
	}
	if r.Tracker.Value != 11000 || r.Tracker.Volume != 10000 {
		t.Errorf("tracker = %+v, want value 11000 at volume 10000", r.Tracker)
	}
	for _, want := range []string{`"expires_at": null`, `"dynamic": true`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("document does not contain %s:\n%s", want, body)
		}
	}
	if i, j := strings.Index(string(body), `"penalty"`), strings.Index(string(body), `"dynamic"`); i < 0 || j < i {
		t.Errorf("dynamic key does not follow penalty in the document:\n%s", body)
	}
}

// TestE2EDynamicReportDiesAtTheStrictBoundary: the report is alive
// strictly above its recorded volume and dead at it (value <= volume),
// with no timer - the decay of the tracker value is the only clock.
func TestE2EDynamicReportDiesAtTheStrictBoundary(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicRules)

	sendAt(d, "1760724000", "203.0.113.7", "/releases/24.04.iso", 6000)
	sendAt(d, "1760724001", "203.0.113.7", "/releases/24.04.iso", 6000)
	d.waitProcessed(t, 2)

	// 18:00:01: value 11000, strictly above volume 10000 - alive.
	clock.Advance(time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 {
		t.Fatalf("reports at 11000 = %+v, want 1 (strictly above volume)", doc.Reports)
	}

	// 18:00:02: the value decays to exactly 10000 = volume - dead at
	// the strict boundary.
	clock.Advance(time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports at 10000 = %+v, want none (dead at value <= volume)", doc.Reports)
	}
}

// TestE2EDynamicReportReflagsWithinAnEvent: once the death has been
// served, renewed overflow re-flags a fresh report within an event -
// the very event that overflows writes it.
func TestE2EDynamicReportReflagsWithinAnEvent(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicRules)

	sendAt(d, "1760724000", "203.0.113.7", "/releases/24.04.iso", 6000)
	sendAt(d, "1760724001", "203.0.113.7", "/releases/24.04.iso", 6000)
	d.waitProcessed(t, 2)

	// 18:00:02: decayed to the line - the death is served.
	clock.Advance(2 * time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports at the death line = %+v, want none", doc.Reports)
	}

	// Traffic flows again: 11000 + 6000 - 1000*1s = 16000 overflows at
	// the first renewal event, which writes the fresh report.
	sendAt(d, "1760724002", "203.0.113.7", "/releases/24.04.iso", 6000)
	d.waitProcessed(t, 3)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after the first renewal event = %+v, want 1 (re-flagged within an event)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.UpdatedAt != "2025-10-17T18:00:02Z" {
		t.Errorf("updated_at = %q, want the renewal write %q", r.UpdatedAt, "2025-10-17T18:00:02Z")
	}
	if r.Tracker.Value != 16000 {
		t.Errorf("tracker value = %v, want 16000 (the accumulation as it stands)", r.Tracker.Value)
	}
	if r.CreatedAt != wallBase {
		t.Errorf("created_at = %q, want %q (upsert identity over the still-stored row)", r.CreatedAt, wallBase)
	}
}

// TestE2EDynamicSweepDeletesTheDeadRow: the death is materialized at
// sweep time too - the swept row goes, and the next overflow writes a
// fresh report at the accumulation's current stance.
func TestE2EDynamicSweepDeletesTheDeadRow(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicRules)

	sendAt(d, "1760724000", "203.0.113.7", "/releases/24.04.iso", 6000)
	sendAt(d, "1760724001", "203.0.113.7", "/releases/24.04.iso", 6000)
	d.waitProcessed(t, 2)

	// 18:00:02: dead at the line; no document in between - the sweep
	// materializes the death.
	clock.Advance(2 * time.Second)
	if n, err := d.sweepOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("sweepOnce = (%d, %v), want (1, nil)", n, err)
	}

	sendAt(d, "1760724002", "203.0.113.7", "/big2", 6000)
	d.waitProcessed(t, 3)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after re-flag = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.CreatedAt != "2025-10-17T18:00:02Z" || r.UpdatedAt != "2025-10-17T18:00:02Z" {
		t.Errorf("created_at/updated_at = %q/%q, want a fresh %q write",
			r.CreatedAt, r.UpdatedAt, "2025-10-17T18:00:02Z")
	}
	if r.Sample.Path != "/big2" {
		t.Errorf("sample path = %q, want the writing event's /big2", r.Sample.Path)
	}
}

// TestE2EDynamicSuppressionFreezesReportButRunsActions: while the value
// stays above the volume further Triggers write nothing - the report
// freezes at its first write - but the actions keep running.
func TestE2EDynamicSuppressionFreezesReportButRunActions(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicRules)

	// 6000, 11000, 16000, 21000: the second event writes, the third and
	// fourth overflow into suppression. Every overflowing event runs
	// the report action's sibling log action.
	for i := range 4 {
		sendAt(d, fmt.Sprintf("%d", 1760724000+i), "203.0.113.7", "/releases/24.04.iso", 6000)
		d.waitProcessed(t, int64(i+1))
		if i < 3 {
			clock.Advance(time.Second)
		}
	}

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one (one write while alive)", doc.Reports)
	}
	r := doc.Reports[0]
	if r.CreatedAt != "2025-10-17T18:00:01Z" || r.UpdatedAt != "2025-10-17T18:00:01Z" {
		t.Errorf("created_at/updated_at = %q/%q, want the 18:00:01 write frozen (suppressed)",
			r.CreatedAt, r.UpdatedAt)
	}
	if r.Tracker.Value != 11000 {
		t.Errorf("tracker value = %v, want 11000 (the first write's reading, frozen)", r.Tracker.Value)
	}
	if r.Sample.Time != "2025-10-17T18:00:01Z" {
		t.Errorf("sample time = %q, want the first write's event (frozen)", r.Sample.Time)
	}
	if got := marks(d); len(got) != 3 {
		t.Errorf("log-action marks = %v, want 3 (every overflowing event runs its actions)", got)
	}
}

// TestE2EDynamicLadderOverrideInPlace: a strictly higher rung overrides
// the live dynamic report in place - the stance moves to the top
// crossed rung, created_at alone stays frozen - while an at-or-below
// rung writes nothing.
func TestE2EDynamicLadderOverrideInPlace(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicLadderRules)

	sendAt(d, "1760724000", "203.0.113.7", "/big", 100)
	d.waitProcessed(t, 1)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.throttle" {
		t.Fatalf("reports = %+v, want the throttle rung's write", doc.Reports)
	}

	// 100 + 100 - 1*30 = 170: both rungs refire. The throttle rung is
	// at the recorded volume and writes nothing; the ban rung overrides
	// in place.
	clock.Advance(30 * time.Second) // 18:00:30
	sendAt(d, "1760724030", "203.0.113.7", "/big2", 100)
	d.waitProcessed(t, 2)

	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want still exactly one", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.ban" || r.Penalty == nil || r.Penalty.Action != "drop" {
		t.Errorf("stance = %q %+v, want bandage.ban/drop (the overriding rung's)", r.Reason, r.Penalty)
	}
	if r.Tracker.Value != 170 || r.Tracker.Volume != 150 {
		t.Errorf("tracker = %+v, want value 170 at the overriding rung's threshold 150", r.Tracker)
	}
	if r.CreatedAt != wallBase {
		t.Errorf("created_at = %q, want %q (frozen at first flagged)", r.CreatedAt, wallBase)
	}
	if r.UpdatedAt != "2025-10-17T18:00:30Z" {
		t.Errorf("updated_at = %q, want the overriding write", r.UpdatedAt)
	}
}

// TestE2EDynamicFailedWriteRetries: a failed write is not a Trigger
// outcome - the next matching event retries and lands the report.
func TestE2EDynamicFailedWriteRetries(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	inner, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store := &failingStore{ReportRepository: inner, failOn: 1}
	d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", dynamicRules, nil, withStore(store))

	// 15000 > 10000 overflows on the first event; its write fails. The
	// second event (15000 + 15000 - 1000*1s = 29000) retries and lands.
	sendAt(d, "1760724000", "203.0.113.7", "/big", 15000)
	d.waitProcessed(t, 1)
	clock.Advance(time.Second)
	sendAt(d, "1760724001", "203.0.113.7", "/big", 15000)
	d.waitProcessed(t, 2)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want the retried write", doc.Reports)
	}
	r := doc.Reports[0]
	if r.UpdatedAt != "2025-10-17T18:00:01Z" {
		t.Errorf("updated_at = %q, want the %q retry write", r.UpdatedAt, "2025-10-17T18:00:01Z")
	}
	if r.Tracker.Value != 29000 {
		t.Errorf("tracker value = %v, want 29000 (the retry's reading)", r.Tracker.Value)
	}
}

// dynamicReplayRules is the dynamic replay form: the writing key is the
// (Target, path) record that justified the report.
const dynamicReplayRules = `
version: 0
chains:
  - name: main
    rules:
      - id: enrich-size
        actions: [{enrich: {type: filesize}}]
      - id: iso-replay
        tracker: {track: replay, leak: 1, volume: 1}
        actions: [{report: {reason: iso-replay, penalty: limit-bandwidth, rate: 1MB}}]
`

// TestE2EDynamicReplayDiesWithItsWritingRecord: a dynamic replay report
// is alive while the (Target, path) record that wrote it holds value,
// and dies when that record decays to its volume.
func TestE2EDynamicReplayDiesWithItsWritingRecord(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", dynamicReplayRules)

	// Two full downloads in one event second: 1 + 1 = 2 > 1 overflows.
	for range 2 {
		d.send(nginxLineClen("1760724000", "203.0.113.7", "GET", "/small.iso", 200, 100, "100"))
	}
	d.waitProcessed(t, 2)
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want 1", doc.Reports)
	}

	// The writing record decays 1/s: at 18:00:01 its value is 1, dead
	// at the strict boundary.
	clock.Advance(time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports = %+v, want none (the writing record decayed to volume)", doc.Reports)
	}
}

// mixedLifecycleRules mixes fixed and dynamic report actions on one
// chain: every event feeds both trackers, so one restart sweeps the
// dynamic reports and keeps the fixed ones.
const mixedLifecycleRules = `
version: 0
chains:
  - name: main
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 1000, volume: 10000}
        actions: [{report: {reason: egress-overflow, penalty: limit-bandwidth, rate: 1MB}}]
      - id: sticky
        tracker: {track: bandwidth, leak: 0, volume: 100}
        actions: [{report: {reason: sticky-overflow, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`

// reportEntryFor returns the document entry for (rule, target), or the
// zero entry with ok false.
func reportEntryFor(doc reportDoc, rule, target string) (reportEntry, bool) {
	for _, r := range doc.Reports {
		if r.Rule == rule && r.Target == target {
			return r, true
		}
	}
	return reportEntry{}, false
}

// TestE2ERestartSweepsDynamicReports: restarting a daemon over an
// existing database removes every dynamic report and keeps every fixed
// one; suppression is rebuilt from the fixed rows only, so a re-flagged
// dynamic target writes fresh (spec: Startup sweep).
func TestE2ERestartSweepsDynamicReports(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	dbPath := filepath.Join(t.TempDir(), "reports.db")

	// Two targets overflow both rules: two dynamic (egress) and two
	// fixed (sticky) reports, all written at 18:00:00.
	d1 := startTestDaemon(t, clock, "unixgram", dbPath, mixedLifecycleRules)
	for _, client := range []string{"203.0.113.7", "203.0.113.8"} {
		sendAt(d1, "1760724000", client, "/big", 6000)
		sendAt(d1, "1760724001", client, "/big", 6000)
	}
	d1.waitProcessed(t, 4)
	if doc := d1.getReports(t, nil); len(doc.Reports) != 4 {
		t.Fatalf("before restart: %+v, want 2 dynamic + 2 fixed", doc.Reports)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := d1.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	cancel()

	// The restart sweeps the two dynamic rows before the live-report set
	// is rebuilt; the two fixed rows survive untouched.
	clock.Advance(10 * time.Minute) // 18:10
	d2 := startTestDaemon(t, clock, "unixgram", dbPath, mixedLifecycleRules)
	doc := d2.getReports(t, nil)
	if len(doc.Reports) != 2 {
		t.Fatalf("after restart: %+v, want the 2 fixed reports only", doc.Reports)
	}
	for _, client := range []string{"203.0.113.7/32", "203.0.113.8/32"} {
		r, ok := reportEntryFor(doc, "sticky", client)
		if !ok {
			t.Fatalf("after restart: %+v, want the fixed report for %s kept", doc.Reports, client)
		}
		if r.Dynamic || r.CreatedAt != wallBase || r.UpdatedAt != wallBase || r.Sample.Path != "/big" {
			t.Errorf("fixed report for %s = %+v, want the pre-restart write kept as-is", client, r)
		}
	}

	// Suppression is rebuilt from the fixed rows only: the sticky rule
	// stays frozen while the swept egress target re-flags fresh on
	// renewed overflow (6000 + 6000 - 1000*1s = 11000 > 10000).
	sendAt(d2, "1760724600", "203.0.113.7", "/big2", 6000)
	d2.waitProcessed(t, 1)
	clock.Advance(time.Second) // 18:10:01
	sendAt(d2, "1760724601", "203.0.113.7", "/big2", 6000)
	d2.waitProcessed(t, 2)

	doc = d2.getReports(t, nil)
	if len(doc.Reports) != 3 {
		t.Fatalf("after re-flag: %+v, want 2 fixed + 1 fresh dynamic", doc.Reports)
	}
	r, ok := reportEntryFor(doc, "sticky", "203.0.113.7/32")
	if !ok || r.UpdatedAt != wallBase || r.Sample.Path != "/big" {
		t.Errorf("sticky report = %+v (ok %v), want it frozen at the pre-restart write (fixed rows suppress)", r, ok)
	}
	r, ok = reportEntryFor(doc, "egress", "203.0.113.7/32")
	if !ok {
		t.Fatalf("after re-flag: %+v, want the re-flagged egress report", doc.Reports)
	}
	if !r.Dynamic || r.CreatedAt != "2025-10-17T18:10:01Z" || r.UpdatedAt != "2025-10-17T18:10:01Z" {
		t.Errorf("egress report = %+v, want a fresh 18:10:01 write (the swept row writes fresh)", r)
	}
	if r.Tracker.Value != 11000 || r.Tracker.Volume != 10000 {
		t.Errorf("egress tracker = %+v, want value 11000 at volume 10000 (the restart-cold re-accumulation)", r.Tracker)
	}
	if _, ok := reportEntryFor(doc, "egress", "203.0.113.8/32"); ok {
		t.Errorf("egress report for 203.0.113.8/32 = present, want swept and gone: %+v", doc.Reports)
	}
}

// TestE2EStartupSweepLogsOneSummaryLine: the startup sweep reports
// itself as one summary line carrying the count - its whole voice above
// debug, nothing per-report (spec: Observability).
func TestE2EStartupSweepLogsOneSummaryLine(t *testing.T) {
	rules := `
version: 0
chains:
  - name: main
    rules:
      - id: burst
        tracker: {track: bandwidth, leak: 1, volume: 100}
        actions: [{report: {reason: burst-overflow, penalty: limit-bandwidth, rate: 1MB}}]
`
	clock := newTestClock(mustTime(t, wallBase))
	dbPath := filepath.Join(t.TempDir(), "reports.db")

	// One overflowing event per target: two dynamic reports to sweep.
	d1 := startTestDaemon(t, clock, "unixgram", dbPath, rules)
	sendAt(d1, "1760724000", "203.0.113.7", "/big", 150)
	sendAt(d1, "1760724000", "203.0.113.8", "/big", 150)
	d1.waitProcessed(t, 2)
	if doc := d1.getReports(t, nil); len(doc.Reports) != 2 {
		t.Fatalf("before restart: %+v, want the 2 dynamic reports", doc.Reports)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := d1.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	cancel()

	d2 := startTestDaemon(t, clock, "unixgram", dbPath, rules)

	recs := d2.logs.withMessage("startup sweep: dynamic reports deleted")
	if len(recs) != 1 {
		t.Fatalf("startup sweep logs = %v, want exactly one summary line", recs)
	}
	if recs[0].Level != zerolog.InfoLevel {
		t.Errorf("summary level = %v, want info (once per start, not steady-state noise)", recs[0].Level)
	}
	var n int
	attrInto(t, recs[0], "count", &n)
	if n != 2 {
		t.Errorf("summary count = %d, want 2 (the swept dynamic reports)", n)
	}

	// Nothing per-report above debug: the startup's non-debug records
	// are the summary and the daemon's startup announcements.
	var msgs []string
	for _, rec := range d2.logs.atAndAbove(zerolog.InfoLevel) {
		msgs = append(msgs, rec.Message)
	}
	slices.Sort(msgs)
	want := []string{"api started", "starting", "startup sweep: dynamic reports deleted", "syslog ingest started"}
	if !slices.Equal(msgs, want) {
		t.Errorf("non-debug startup records = %v, want %v", msgs, want)
	}
}
