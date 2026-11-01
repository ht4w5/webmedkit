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
// expiry or deletion. Identity is (rule, target).
type Report struct {
	Rule      string
	Chain     string
	Reason    string
	Target    string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
	Tracker   TrackerReading
	// Sample is the most recent Event that created the report, carried
	// whole (ADR 0021): there is no sample-specific field list to rot.
	Sample Event
	// Penalty is the advisory enforcement stance the report recommends
	// for the target (ADR 0010); nil only for rows written before the
	// penalty existed (outside the canonical contract).
	Penalty *Penalty
}

// NamespacedReason composes a Reason's namespaced form (GLOSSARY): prefix, dot,
// authored code (ADR 0016, ADR 0017) -- the one join, so plan and report agree.
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
// absent sample fields read as zero (zero means absent).
func (e *Event) UnmarshalJSON(b []byte) error {
	var w sampleJSON
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	t, err := parseTime(w.Time)
	if err != nil {
		return err
	}
	ev := Event(w.sampleWire)
	ev.Time = t
	*e = ev
	return nil
}

// MarshalJSON renders the report in the report document shape.
func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Rule      string         `json:"rule"`
		Chain     string         `json:"chain"`
		Reason    string         `json:"reason"`
		Target    string         `json:"target"`
		CreatedAt string         `json:"created_at"`
		UpdatedAt string         `json:"updated_at"`
		ExpiresAt string         `json:"expires_at"`
		Tracker   TrackerReading `json:"tracker"`
		Sample    Event          `json:"sample"`
		Penalty   *Penalty       `json:"penalty,omitempty"`
	}{
		Rule:      r.Rule,
		Chain:     r.Chain,
		Reason:    r.Reason,
		Target:    r.Target,
		CreatedAt: formatTime(r.CreatedAt),
		UpdatedAt: formatTime(r.UpdatedAt),
		ExpiresAt: formatTime(r.ExpiresAt),
		Tracker:   r.Tracker,
		Sample:    r.Sample,
		Penalty:   r.Penalty,
	})
}
