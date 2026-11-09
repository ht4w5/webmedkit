package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Downgrade through ladders at the daemon seam (spec: Downgrade): a
// dynamic ladder report restates itself to the highest rung still
// exceeded as its tracker value decays - reason, Penalty and updated_at
// moving to the rung's values, created_at frozen, the Sample staying
// the climb event - and dies when no rung remains exceeded. Event
// times sit on the injected wall clock's timeline, so clock advance is
// the decay driver.

// downgradeLadderRules is a two-rung dynamic ladder: neither report
// action carries a ttl, so the whole ladder decays with the value.
const downgradeLadderRules = `
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

// TestE2EDynamicBanDowngradesToTheThrottleRung: as the value decays to
// the ban stance's death line with the throttle rung still exceeded,
// the report demotes to the throttle rung's stance - carrying the
// lower rung's reason and Penalty over the climb event's Sample -
// with created_at frozen and updated_at moved.
func TestE2EDynamicBanDowngradesToTheThrottleRung(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", downgradeLadderRules)

	// One 200-byte download overflows both rungs; the ban rung's write
	// lands last and the report stands at the ban stance.
	sendAt(d, "1760724000", "203.0.113.7", "/big", 200)
	d.waitProcessed(t, 1)

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %+v, want exactly one", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.ban" || r.Penalty == nil || r.Penalty.Action != "drop" {
		t.Fatalf("stance = %q %+v, want bandage.ban/drop (the top crossed rung)", r.Reason, r.Penalty)
	}

	// 18:00:49: value 151, strictly above the ban rung's threshold
	// 150 - the stance stands.
	clock.Advance(49 * time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.ban" {
		t.Fatalf("reports at 151 = %+v, want the ban stance standing", doc.Reports)
	}

	// 18:00:50: the value decays to 150 - the ban stance's line is
	// crossed and the throttle rung is still exceeded: demote.
	clock.Advance(time.Second)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports at 150 = %+v, want 1 (the throttle rung is still exceeded)", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "bandage.throttle" {
		t.Errorf("reason = %q, want bandage.throttle (the lower rung's)", r.Reason)
	}
	if r.Penalty == nil || r.Penalty.Action != "limit-bandwidth" || r.Penalty.Rate == nil || *r.Penalty.Rate != 1000000 {
		t.Errorf("penalty = %+v, want the throttle rung's limit-bandwidth rate 1MB", r.Penalty)
	}
	if r.CreatedAt != wallBase {
		t.Errorf("created_at = %q, want %q (frozen at first flagged)", r.CreatedAt, wallBase)
	}
	if r.UpdatedAt != "2025-10-17T18:00:50Z" {
		t.Errorf("updated_at = %q, want the downgrade write", r.UpdatedAt)
	}
	if !r.Dynamic || r.ExpiresAt != "" {
		t.Errorf("dynamic/expires_at = %v/%q, want a dynamic entry with null expiry (the rung's policy)", r.Dynamic, r.ExpiresAt)
	}
	// The reading restates to the rung: its threshold as volume, the
	// climb's recorded value otherwise.
	if r.Tracker.Value != 200 || r.Tracker.Leak != 1 || r.Tracker.Volume != 50 {
		t.Errorf("tracker = %+v, want value 200 at the throttle rung's threshold 50", r.Tracker)
	}
	// The Sample backing the demoted stance is the climb event, which
	// crossed every lower rung.
	checkSample(t, r.Sample, sampleEntry{
		Time: "2025-10-17T18:00:00Z", RemoteAddr: "203.0.113.7", Method: "GET",
		Path: "/big", Status: 200, SentBytes: 200,
	})
}

// downgradeChainRules is a three-rung dynamic ladder with grace on the
// lowest rung: its death line is 50 - leak x grace = 50 - 30 = 20.
const downgradeChainRules = `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, grace: 30s, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 100
            actions: [{report: {reason: squeeze, penalty: limit-req-rate, rate: 50}}]
          - at: 150
            actions: [{report: {reason: ban, penalty: drop}}]
`

// TestE2EDynamicDowngradeChainsAndDiesBelowTheLowestRung: the report
// demotes rung by rung as the value decays - created_at frozen across
// every downgrade, updated_at moving - and dies when the value decays
// below the lowest rung minus the stance's margin.
func TestE2EDynamicDowngradeChainsAndDiesBelowTheLowestRung(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", downgradeChainRules)

	sendAt(d, "1760724000", "203.0.113.7", "/big", 160)
	d.waitProcessed(t, 1)

	// 18:00:10: value 150 - the ban stance's line, demote to squeeze.
	clock.Advance(10 * time.Second)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.squeeze" {
		t.Fatalf("reports at 150 = %+v, want the squeeze stance", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Penalty == nil || r.Penalty.Action != "limit-req-rate" || r.Penalty.Rate == nil || *r.Penalty.Rate != 50 {
		t.Errorf("penalty = %+v, want the squeeze rung's limit-req-rate rate 50", r.Penalty)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T18:00:10Z" {
		t.Errorf("created_at/updated_at = %q/%q, want %q frozen with the downgrade write moving",
			r.CreatedAt, r.UpdatedAt, wallBase)
	}
	if r.Tracker.Volume != 100 {
		t.Errorf("tracker volume = %v, want the squeeze rung's threshold 100", r.Tracker.Volume)
	}

	// 18:01:00: value 100 - the squeeze stance's line, demote to
	// throttle. created_at stays frozen across the second downgrade.
	clock.Advance(50 * time.Second)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.throttle" {
		t.Fatalf("reports at 100 = %+v, want the throttle stance", doc.Reports)
	}
	r = doc.Reports[0]
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T18:01:00Z" {
		t.Errorf("created_at/updated_at = %q/%q, want %q frozen across downgrades with the write moving",
			r.CreatedAt, r.UpdatedAt, wallBase)
	}
	if r.Tracker.Volume != 50 {
		t.Errorf("tracker volume = %v, want the throttle rung's threshold 50", r.Tracker.Volume)
	}

	// 18:02:19: value 21 - above the throttle stance's widened line
	// 50 - leak x grace = 20: alive.
	clock.Advance(79 * time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 {
		t.Fatalf("reports at 21 = %+v, want 1 (above the lowest rung minus its margin)", doc.Reports)
	}

	// 18:02:20: value 20 - below the lowest rung minus the stance's
	// margin: no rung remains exceeded, the report dies.
	clock.Advance(time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 0 {
		t.Fatalf("reports at 20 = %+v, want none (dead below the lowest rung minus its margin)", doc.Reports)
	}
}

// TestE2EFixedStancesIgnoreDecay: a fixed stance never decay-interacts -
// no Downgrade, no underflow death - in single volume rules and ladder
// rungs alike (spec: Downgrade).
func TestE2EFixedStancesIgnoreDecay(t *testing.T) {
	// fixedSingleRules is the single-volume fixed form: a ttl-bounded
	// report immune to the value's decay.
	const fixedSingleRules = `
version: 0
chains:
  - name: main
    rules:
      - id: egress
        tracker: {track: bandwidth, leak: 1000, volume: 10000}
        actions: [{report: {reason: egress-overflow, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
`
	t.Run("single volume rule", func(t *testing.T) {
		clock := newTestClock(mustTime(t, wallBase))
		d := startTestDaemon(t, clock, "unixgram", ":memory:", fixedSingleRules)

		// 6000 + 6000 - 1000*1s = 11000 > 10000: the second event
		// overflows and writes the fixed report.
		sendAt(d, "1760724000", "203.0.113.7", "/big", 6000)
		sendAt(d, "1760724001", "203.0.113.7", "/big", 6000)
		d.waitProcessed(t, 2)

		// The value decays to 0 - far below the volume. A dynamic report
		// would die at 18:00:11; the fixed one stands until its ttl.
		clock.Advance(20 * time.Second)
		doc := d.getReports(t, nil)
		if len(doc.Reports) != 1 {
			t.Fatalf("reports after the decay = %+v, want the fixed report standing", doc.Reports)
		}
		r := doc.Reports[0]
		if r.Dynamic || r.ExpiresAt != "2025-10-17T19:00:00Z" || r.UpdatedAt != wallBase {
			t.Errorf("report = %+v, want the fixed 18:00:00 write with its 1h ttl untouched by decay", r)
		}
	})

	// fixedLadderRules is the mixed ladder with a fixed ban rung on top
	// of a dynamic throttle rung.
	const fixedLadderRules = `
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
            actions: [{report: {reason: ban, ttl: 1h, penalty: drop}}]
`
	t.Run("ladder rung", func(t *testing.T) {
		clock := newTestClock(mustTime(t, wallBase))
		d := startTestDaemon(t, clock, "unixgram", ":memory:", fixedLadderRules)

		sendAt(d, "1760724000", "203.0.113.7", "/big", 200)
		d.waitProcessed(t, 1)

		// The value decays to nothing - below every rung. The fixed ban
		// stance neither downgrades to the dynamic throttle rung nor
		// dies at underflow: its policy is the writing action's ttl.
		clock.Advance(200 * time.Second)
		doc := d.getReports(t, nil)
		if len(doc.Reports) != 1 {
			t.Fatalf("reports after the decay = %+v, want the fixed ban stance standing", doc.Reports)
		}
		r := doc.Reports[0]
		if r.Reason != "bandage.ban" || r.Dynamic || r.ExpiresAt != "2025-10-17T19:00:00Z" || r.UpdatedAt != wallBase {
			t.Errorf("report = %+v, want the fixed ban stance untouched by decay", r)
		}
	})
}

// downgradeGraceRules is a two-rung dynamic ladder with grace on both
// rungs: the ban stance's line is 150 - 20 = 130, the throttle
// stance's 50 - 30 = 20.
const downgradeGraceRules = `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, grace: 30s, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, grace: 20s, penalty: drop}}]
`

// TestE2EDynamicDowngradeMarginGuardsTheBoundary: the margin guards
// the downgrade line too, so stances do not flap between rungs at the
// boundary - the ban stance stands inside its margin band below the
// rung's threshold, and once demoted the stance stays put until the
// value climbs back above the top rung (no re-promotion from a bump
// inside the band).
func TestE2EDynamicDowngradeMarginGuardsTheBoundary(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", downgradeGraceRules)

	sendAt(d, "1760724000", "203.0.113.7", "/big", 200)
	d.waitProcessed(t, 1)

	// 18:01:09: value 131 - below the ban rung's threshold 150 but
	// above its widened line 130: the margin keeps the ban stance.
	clock.Advance(69 * time.Second)
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.ban" {
		t.Fatalf("reports at 131 = %+v, want the ban stance (inside the margin band)", doc.Reports)
	}

	// 18:01:10: value 150 - 20 = 130 - the widened downgrade line,
	// strict: demote to throttle.
	clock.Advance(time.Second)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.throttle" {
		t.Fatalf("reports at 130 = %+v, want the throttle stance", doc.Reports)
	}

	// A one-byte bump parks the value at 131 - back inside the band,
	// still below the top rung. The downgraded stance stays: the at-
	// or-below Trigger writes nothing, and no boundary bump re-promotes.
	sendAt(d, "1760724070", "203.0.113.7", "/small", 1)
	d.waitProcessed(t, 2)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after the bump = %+v, want 1", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.throttle" || r.UpdatedAt != "2025-10-17T18:01:10Z" {
		t.Errorf("report = %+v, want the demoted stance frozen at the downgrade write (no boundary flap)", r)
	}
	checkSample(t, r.Sample, sampleEntry{
		Time: "2025-10-17T18:00:00Z", RemoteAddr: "203.0.113.7", Method: "GET",
		Path: "/big", Status: 200, SentBytes: 200,
	})

	// Re-escalation rides the Trigger path unchanged: a strictly higher
	// rung overrides in place, only the value climbing back above the
	// top rung promotes (story 18).
	clock.Advance(time.Second) // 18:01:11
	sendAt(d, "1760724071", "203.0.113.7", "/big2", 100)
	d.waitProcessed(t, 3)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after the re-climb = %+v, want 1", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "bandage.ban" || r.Penalty == nil || r.Penalty.Action != "drop" {
		t.Errorf("stance = %q %+v, want bandage.ban/drop (re-escalated in place)", r.Reason, r.Penalty)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T18:01:11Z" {
		t.Errorf("created_at/updated_at = %q/%q, want %q frozen with the overriding write moving",
			r.CreatedAt, r.UpdatedAt, wallBase)
	}
	if r.Tracker.Value != 230 || r.Tracker.Volume != 150 {
		t.Errorf("tracker = %+v, want value 230 at the overriding rung's threshold 150", r.Tracker)
	}
	checkSample(t, r.Sample, sampleEntry{
		Time: "2025-10-17T18:01:11Z", RemoteAddr: "203.0.113.7", Method: "GET",
		Path: "/big2", Status: 200, SentBytes: 100,
	})
}

// demote drives one downgrade at the daemon seam: the climb overflows
// both rungs of downgradeLadderRules, then the decay to the ban
// stance's line demotes the report at the next document.
func demote(t *testing.T, clock *testClock, d *testDaemon) {
	t.Helper()
	sendAt(d, "1760724000", "203.0.113.7", "/big", 200)
	d.waitProcessed(t, 1)
	clock.Advance(50 * time.Second) // 18:00:50: value 150 - the ban line
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.throttle" {
		t.Fatalf("reports at 150 = %+v, want the demoted throttle stance", doc.Reports)
	}
}

// TestE2EDowngradeLoggedAtDebugLevel: a Downgrade act appears only at
// debug log level (spec: Observability) - steady-state lifecycle acts
// never flood the log.
func TestE2EDowngradeLoggedAtDebugLevel(t *testing.T) {
	// The act itself is a debug record.
	t.Run("record", func(t *testing.T) {
		clock := newTestClock(mustTime(t, wallBase))
		d := startTestDaemon(t, clock, "unixgram", ":memory:", downgradeLadderRules)
		demote(t, clock, d)

		downgrades := d.logs.withMessage("report downgraded")
		if len(downgrades) != 1 {
			t.Fatalf("downgrade logs = %v, want 1 (logged per act)", downgrades)
		}
		if rec := downgrades[0]; rec.Level != zerolog.DebugLevel {
			t.Errorf("downgrade level = %v, want debug (demote is steady state)", rec.Level)
		}
	})

	// A configured level shows demote acts at debug and silences them
	// at info: they appear only at debug.
	for _, tt := range []struct {
		level domain.Level
		want  bool
	}{
		{level: domain.LevelDebug, want: true},
		{level: domain.LevelInfo, want: false},
	} {
		t.Run(tt.level.String(), func(t *testing.T) {
			clock := newTestClock(mustTime(t, wallBase))
			buf := &syncBuffer{}
			d := startTestDaemonCfg(t, clock, "unixgram", ":memory:", downgradeLadderRules, nil, WithLogger(NewLogger(buf, tt.level)))
			demote(t, clock, d)

			line := ""
			for _, l := range strings.Split(buf.String(), "\n") {
				if strings.Contains(l, "report downgraded") {
					line = l
					break
				}
			}
			if (line != "") != tt.want {
				t.Errorf("demote lines at %s level: present = %v, want %v:\n%s", tt.level, line != "", tt.want, buf.String())
			}
		})
	}
}

// downgradeMixedRules mixes lifecycle policies on one ladder: a
// dynamic ban rung (grace 20s, line 130) over a fixed throttle rung
// (ttl 1h) - the policy attaches to the rung that writes the stance.
const downgradeMixedRules = `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, grace: 20s, penalty: drop}}]
`

// TestE2EDynamicDowngradeOntoFixedRungStampsTheTTL: downgrading onto a
// fixed rung stamps that rung's ttl from the downgrade write, and the
// stance then behaves as a fixed report - no further decay interaction
// - so the mixed ladder behaves per-rung (spec: Downgrade).
func TestE2EDynamicDowngradeOntoFixedRungStampsTheTTL(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", downgradeMixedRules)

	sendAt(d, "1760724000", "203.0.113.7", "/big", 200)
	d.waitProcessed(t, 1)
	if doc := d.getReports(t, nil); len(doc.Reports) != 1 || doc.Reports[0].Reason != "bandage.ban" || !doc.Reports[0].Dynamic {
		t.Fatalf("reports = %+v, want the dynamic ban stance", doc.Reports)
	}

	// 18:01:10: value 130 - the ban stance's widened line: demote onto
	// the fixed throttle rung, whose 1h ttl runs from this write.
	clock.Advance(70 * time.Second)
	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports at 130 = %+v, want the throttle stance", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.throttle" {
		t.Errorf("reason = %q, want bandage.throttle (the fixed rung's stance)", r.Reason)
	}
	if r.Penalty == nil || r.Penalty.Action != "limit-bandwidth" || r.Penalty.Rate == nil || *r.Penalty.Rate != 1000000 {
		t.Errorf("penalty = %+v, want the throttle rung's limit-bandwidth rate 1MB", r.Penalty)
	}
	if r.Dynamic || r.ExpiresAt != "2025-10-17T19:01:10Z" {
		t.Errorf("dynamic/expires_at = %v/%q, want a fixed entry with the ttl stamped from the 18:01:10 write", r.Dynamic, r.ExpiresAt)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T18:01:10Z" {
		t.Errorf("created_at/updated_at = %q/%q, want %q frozen with the downgrade write moving",
			r.CreatedAt, r.UpdatedAt, wallBase)
	}
	if r.Tracker.Value != 200 || r.Tracker.Volume != 50 {
		t.Errorf("tracker = %+v, want value 200 at the throttle rung's threshold 50", r.Tracker)
	}
	checkSample(t, r.Sample, sampleEntry{
		Time: "2025-10-17T18:00:00Z", RemoteAddr: "203.0.113.7", Method: "GET",
		Path: "/big", Status: 200, SentBytes: 200,
	})

	// The stamped stance behaves as a fixed report: the value decays to
	// nothing and it stands until the stamped expiry, never at
	// underflow (per-rung policy on a mixed ladder).
	clock.Advance(3 * time.Minute)
	doc = d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after the decay = %+v, want the stamped fixed stance standing", doc.Reports)
	}
	r = doc.Reports[0]
	if r.Reason != "bandage.throttle" || r.Dynamic || r.ExpiresAt != "2025-10-17T19:01:10Z" || r.UpdatedAt != "2025-10-17T18:01:10Z" {
		t.Errorf("report = %+v, want the stamped fixed stance untouched by decay", r)
	}
}

// TestE2EDynamicSweepMaterializesTheDowngrade: the lifecycle act is
// materialized at sweep time too - a decaying report with a lower rung
// still exceeded is restated by the sweep pass (a downgrade, not a
// deletion), never swept away (spec: Dynamic report liveness).
func TestE2EDynamicSweepMaterializesTheDowngrade(t *testing.T) {
	clock := newTestClock(mustTime(t, wallBase))
	d := startTestDaemon(t, clock, "unixgram", ":memory:", downgradeLadderRules)

	sendAt(d, "1760724000", "203.0.113.7", "/big", 200)
	d.waitProcessed(t, 1)

	// 18:00:50: value 150 - the ban line; no document in between - the
	// sweep materializes the downgrade at its own write.
	clock.Advance(50 * time.Second)
	if n, err := d.sweepOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("sweepOnce = (%d, %v), want (0, nil) (a downgrade, not a deletion)", n, err)
	}

	doc := d.getReports(t, nil)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports after the sweep = %+v, want the restated stance", doc.Reports)
	}
	r := doc.Reports[0]
	if r.Reason != "bandage.throttle" {
		t.Errorf("reason = %q, want bandage.throttle (restated at the sweep)", r.Reason)
	}
	if r.CreatedAt != wallBase || r.UpdatedAt != "2025-10-17T18:00:50Z" {
		t.Errorf("created_at/updated_at = %q/%q, want %q frozen with the sweep's downgrade write moving",
			r.CreatedAt, r.UpdatedAt, wallBase)
	}
	if r.Tracker.Value != 200 || r.Tracker.Volume != 50 {
		t.Errorf("tracker = %+v, want value 200 at the throttle rung's threshold 50", r.Tracker)
	}
	checkSample(t, r.Sample, sampleEntry{
		Time: "2025-10-17T18:00:00Z", RemoteAddr: "203.0.113.7", Method: "GET",
		Path: "/big", Status: 200, SentBytes: 200,
	})
}
