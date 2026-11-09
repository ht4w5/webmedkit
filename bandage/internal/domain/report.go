package domain

import (
	"encoding/json"
	"time"
)

// TrackerReading is the report's tracker reading: the tracker values
// at trigger time, not its Sample.
type TrackerReading struct {
	// Track is the tracker kind: "rate", "bandwidth", or "replay".
	Track  string  `json:"track"`
	Value  float64 `json:"value"`
	Leak   float64 `json:"leak"`
	Volume float64 `json:"volume"`
}

// Report is the record that a rule matched for a target, alive until
// expiry, Underflow of its recorded volume (dynamic reports), or
// deletion. Identity is (rule, target).
type Report struct {
	Rule      string
	Chain     string
	Reason    string
	Target    string
	CreatedAt time.Time
	UpdatedAt time.Time
	// ExpiresAt is the scheduled death of a fixed report; zero for a
	// dynamic report (no scheduled withdrawal, spec: Two lifecycle modes).
	ExpiresAt time.Time
	Tracker   TrackerReading
	// Sample is the most recent Event that created the report, carried
	// whole (ADR 0021): there is no sample-specific field list to rot.
	Sample Event
	// Penalty is the advisory enforcement stance the report recommends
	// for the target (ADR 0010); nil only for rows written before the
	// penalty existed (outside the canonical contract).
	Penalty *Penalty
	// Dynamic marks a dynamic report: its life is bound to the tracker
	// value behind it, not to a ttl (spec: Two lifecycle modes). A
	// report action without ttl writes one.
	Dynamic bool
	// Margin is the writing action's grace margin (leak x grace,
	// compiled at rule build); the death line is Tracker.Volume minus
	// this margin - the strict boundary the value must stay above.
	// Zero means no grace: the strict line at Tracker.Volume. It is
	// record shape, not report-document shape.
	Margin float64
}

// NamespacedReason composes a Reason's namespaced form (GLOSSARY): prefix, dot,
// authored code (ADR 0016, ADR 0026) -- the one join, so plan and report agree.
func NamespacedReason(prefix, code string) string {
	return prefix + "." + code
}

// formatTime renders report timestamps: RFC 3339 UTC at second precision.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// sampleWire is a defined-type shim over Event: the sample's JSON
// form is derived from the Event's field set and tags by construction,
// so the emitted keys can never lag the type (ADR 0021).
type sampleWire Event

// sampleJSON is the sample's rendered form: the derived field set,
// with Time shadowed by its rendered timestamp. The shadow must carry
// the same JSON name ("time") to dominate the derived Time.
type sampleJSON struct {
	Time string `json:"time"` // shadows sampleWire.Time (the timestamp renderer)
	sampleWire
}

// MarshalJSON renders the Event in the report document's sample shape
// (also the persisted report body's sample shape - one wire form).
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(sampleJSON{
		Time:       formatTime(e.Time),
		sampleWire: sampleWire(e),
	})
}

// UnmarshalJSON is the inverse of MarshalJSON (persistence round-trip);
// absent sample fields read as zero (zero means absent), so a missing
// or empty "time" reads as the zero time.
func (e *Event) UnmarshalJSON(b []byte) error {
	var w sampleJSON
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	ev := Event(w.sampleWire)
	if w.Time != "" {
		t, err := parseTime(w.Time)
		if err != nil {
			return err
		}
		ev.Time = t
	}
	*e = ev
	return nil
}

// MarshalJSON renders the report in the report document shape. A
// dynamic entry appends "dynamic": true after "penalty" and renders
// "expires_at" as null; fixed entries stay byte-identical (spec:
// Report document).
func (r Report) MarshalJSON() ([]byte, error) {
	var expiresAt *string
	if !r.Dynamic {
		s := formatTime(r.ExpiresAt)
		expiresAt = &s
	}
	return json.Marshal(struct {
		Rule      string         `json:"rule"`
		Chain     string         `json:"chain"`
		Reason    string         `json:"reason"`
		Target    string         `json:"target"`
		CreatedAt string         `json:"created_at"`
		UpdatedAt string         `json:"updated_at"`
		ExpiresAt *string        `json:"expires_at"`
		Tracker   TrackerReading `json:"tracker"`
		Sample    Event          `json:"sample"`
		Penalty   *Penalty       `json:"penalty,omitempty"`
		Dynamic   bool           `json:"dynamic,omitempty"`
	}{
		Rule:      r.Rule,
		Chain:     r.Chain,
		Reason:    r.Reason,
		Target:    r.Target,
		CreatedAt: formatTime(r.CreatedAt),
		UpdatedAt: formatTime(r.UpdatedAt),
		ExpiresAt: expiresAt,
		Tracker:   r.Tracker,
		Sample:    r.Sample,
		Penalty:   r.Penalty,
		Dynamic:   r.Dynamic,
	})
}
