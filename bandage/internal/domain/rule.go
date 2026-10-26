package domain

import (
	"strconv"
	"time"
)

// TrackKind is a tracker kind: what a tracker accumulates per target.
type TrackKind string

const (
	// TrackRate accumulates requests (flux FREQ-BUCKET).
	TrackRate TrackKind = "rate"
	// TrackBandwidth accumulates sent bytes (flux BYTE-BUCKET).
	TrackBandwidth TrackKind = "bandwidth"
	// TrackReplay accumulates the transferred fraction of an estimated
	// file size (flux FILE-RATIO-BUCKET); keyed per (Target, path).
	TrackReplay TrackKind = "replay"
)

// VolumeMethod computes a tracker's tolerated volume (download
// equivalents) from the estimated file size; only replay configures one.
type VolumeMethod interface {
	Volume(sizeBytes float64) float64
	// String renders the method for the --test plan.
	String() string
}

// TrackerConfig is a rule's leaky-bucket tracker: explicit leak and
// volume with the cache-entry TTL derived as volume/leak.
type TrackerConfig struct {
	Kind TrackKind
	Leak float64
	// Volume is the constant volume: requests (rate), bytes (bandwidth),
	// or tolerated download equivalents (plain scalar replay).
	Volume float64
	// VolumeMethod computes the replay volume from the estimated file
	// size instead of Volume; nil otherwise.
	VolumeMethod VolumeMethod
	// TTL is the cache-entry TTL derived at build (volume/leak); replay
	// derives it per event and leaves this zero.
	TTL time.Duration
	// TargetPrefix is the tracker's target aggregation (defaulted at build).
	TargetPrefix TargetPrefix
}

// VolumeFor returns the tolerated volume for an estimated file size in
// bytes: the volume method's output when configured, Volume otherwise.
func (c *TrackerConfig) VolumeFor(sizeBytes float64) float64 {
	if c.VolumeMethod != nil {
		return c.VolumeMethod.Volume(sizeBytes)
	}
	return c.Volume
}

// VolumeDesc renders the volume configuration for the --test plan.
func (c *TrackerConfig) VolumeDesc() string {
	if c.VolumeMethod != nil {
		return c.VolumeMethod.String()
	}
	return strconv.FormatFloat(c.Volume, 'f', -1, 64)
}

// ReportAction upserts a report for the tracker's target on match. It is
// the only write path to the report table.
type ReportAction struct {
	// Reason is the machine-readable slug feeding downstream ban rules.
	Reason string
	// TTL is the report's expiry since the trigger wall-clock time.
	TTL time.Duration
	// Penalty is the advisory enforcement stance the report recommends
	// for the target (ADR 0010).
	Penalty Penalty
}

// LogAction emits debug output for a match (flux's "level,message"
// shape). It is never terminal.
type LogAction struct {
	// Level is the parsed "level" argument: the four-word vocabulary can
	// never reach a process-exiting level (ADR 0005).
	Level Level
	// Message is the "message" argument (flux's prefix).
	Message string
}

// Action is a step executed when a rule matches; a match's actions run
// in order. Exactly one field is set.
type Action struct {
	Report *ReportAction
	Log    *LogAction
	// Goto names the target chain: it runs, then this chain ends after
	// the rule's remaining actions have run.
	Goto string
	// Jump names the target chain: it runs, then this chain resumes.
	Jump string
	// Stop ends this chain after the rule's remaining actions have run.
	Stop bool
}

// ActionVisitor handles one action variant; Action.Accept dispatches
// each action to its matching method.
type ActionVisitor interface {
	VisitReport(*ReportAction) error
	VisitLog(*LogAction) error
	VisitGoto(target string) error
	VisitJump(target string) error
	VisitStop() error
}

// Accept dispatches the action to the matching visitor method. Exactly
// one field of Action is set; a zero Action visits nothing.
func (a Action) Accept(v ActionVisitor) error {
	switch {
	case a.Report != nil:
		return v.VisitReport(a.Report)
	case a.Log != nil:
		return v.VisitLog(a.Log)
	case a.Goto != "":
		return v.VisitGoto(a.Goto)
	case a.Jump != "":
		return v.VisitJump(a.Jump)
	case a.Stop:
		return v.VisitStop()
	}
	return nil
}

// Matcher decides whether a rule considers an event. The filter module's
// Filter satisfies it; the core stays dependency-free.
type Matcher interface {
	Match(event *Event) bool
}

// Escalation is a rung on one tracker's ladder (GLOSSARY): a volume
// threshold in the tracker's quantity paired with its own ordered
// action list. A rule without a ladder is exactly one implicit rung
// carrying its volume and its action list.
type Escalation struct {
	// At is the rung's volume threshold in the tracker's quantity.
	At float64
	// Actions is the rung's ordered action list.
	Actions []Action
}

// Terminal reports whether the rung's action list ends chain traversal
// after it has run (report, goto or stop actions) - a static property of
// the action list (ADR 0013).
func (e Escalation) Terminal() bool {
	for _, a := range e.Actions {
		if a.Report != nil || a.Goto != "" || a.Stop {
			return true
		}
	}
	return false
}

// TopRung returns the ladder's top rung: the highest threshold the
// accumulation can climb to. ok is false without a ladder - the
// single-volume form has no static top rung (its volume is the
// reading's).
func (r Rule) TopRung() (Escalation, bool) {
	if len(r.Escalations) == 0 {
		return Escalation{}, false
	}
	return r.Escalations[len(r.Escalations)-1], true
}

// Rule is a named condition-action pairing: a filter, at most one tracker
// and ordered actions. It matches when the filter accepts and the tracker (if any) overflows; the ID namespaces tracker state.
type Rule struct {
	ID string
	// FilterSource is the effective filter expression as written
	// ("any" when omitted), kept for the --test plan print.
	FilterSource string
	Filter       Matcher
	Tracker      *TrackerConfig
	Actions      []Action
	// Escalations is the rule's ladder: rungs with strictly ascending
	// thresholds in the tracker's quantity, each with its own action
	// list (GLOSSARY). A rule carries an action list or a ladder, never
	// both; nil is the single-volume form.
	Escalations []Escalation
}

// allRungs is the rule's whole rung set: its ladder, or the one
// implicit rung standing at volume and carrying the rule's action list
// (the single-volume form). volume only shapes the implicit rung's
// threshold; callers that read action lists alone pass zero.
func (r Rule) allRungs(volume float64) []Escalation {
	if len(r.Escalations) > 0 {
		return r.Escalations
	}
	return []Escalation{{At: volume, Actions: r.Actions}}
}

// Rungs resolves one post-accumulation reading to the rungs a match
// fired, decided once before any action runs: every rung whose threshold
// the reading's value stands above, lowest first. The loader's strictly
// ascending thresholds make the fired rungs a prefix of the ladder.
// The implicit rung carries the reading's volume and fires when the
// value stands above it; a tracker-less rule always fires its implicit
// rung (it matches on filter alone).
func (r Rule) Rungs(reading TrackerReading) []Escalation {
	all := r.allRungs(reading.Volume)
	if r.Tracker == nil {
		return all
	}
	var fired []Escalation
	for _, e := range all {
		if e.At < reading.Value {
			fired = append(fired, e)
		}
	}
	return fired
}

// ActionLists returns every ordered action list the rule can run: the
// rule's action list for the single-volume form, else each rung's.
func (r Rule) ActionLists() [][]Action {
	all := r.allRungs(0)
	lists := make([][]Action, 0, len(all))
	for _, e := range all {
		lists = append(lists, e.Actions)
	}
	return lists
}

// Chain is an ordered group of rules traversed for each event.
type Chain struct {
	Name  string
	Rules []Rule
}
