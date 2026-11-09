package domain

import (
	"testing"
)

// Scenario values come from flux's bucket.yaml test vectors.

func TestRecordAccumulate(t *testing.T) {
	type step struct {
		eventTime    int64
		increment    float64
		wantValue    float64
		wantLast     int64
		wantOverflow bool
	}
	tests := []struct {
		name   string
		leak   float64
		volume float64
		steps  []step
	}{
		{
			// bucket.yaml: FREQ-BUCKET leak=0, volume=2 -> third request triggers.
			name:   "rate burst overflows on third event",
			leak:   0,
			volume: 2,
			steps: []step{
				{eventTime: 100, increment: 1, wantValue: 1, wantLast: 100},
				{eventTime: 100, increment: 1, wantValue: 2, wantLast: 100},
				{eventTime: 100, increment: 1, wantValue: 3, wantLast: 100, wantOverflow: true},
			},
		},
		{
			// bucket.yaml: BYTE-BUCKET leak=0, volume=1024, small volume.
			name:   "bandwidth overflow strictly above volume",
			leak:   0,
			volume: 1024,
			steps: []step{
				{eventTime: 100, increment: 600, wantValue: 600, wantLast: 100},
				{eventTime: 100, increment: 424, wantValue: 1024, wantLast: 100}, // == volume, no overflow
				{eventTime: 100, increment: 1, wantValue: 1025, wantLast: 100, wantOverflow: true},
			},
		},
		{
			// bucket.yaml: BYTE-BUCKET leak=512, volume=2048.
			name:   "decay between events",
			leak:   512,
			volume: 2048,
			steps: []step{
				{eventTime: 100, increment: 1000, wantValue: 1000, wantLast: 100},
				{eventTime: 102, increment: 1000, wantValue: 976, wantLast: 102},  // 2000 - 512*2
				{eventTime: 110, increment: 1000, wantValue: 0, wantLast: 110},    // floored at zero
				{eventTime: 111, increment: 2500, wantValue: 1988, wantLast: 111}, // 2500 - 512
				{eventTime: 111, increment: 100, wantValue: 2088, wantLast: 111, wantOverflow: true},
			},
		},
		{
			// bucket.yaml: FREQ-BUCKET leak=1, volume=4.
			name:   "steady single events drain",
			leak:   1,
			volume: 4,
			steps: []step{
				{eventTime: 100, increment: 1, wantValue: 1, wantLast: 100},
				{eventTime: 101, increment: 1, wantValue: 1, wantLast: 101}, // 2 - 1
				{eventTime: 102, increment: 1, wantValue: 1, wantLast: 102},
			},
		},
		{
			name:   "out-of-order events increment without decay",
			leak:   1,
			volume: 100,
			steps: []step{
				{eventTime: 100, increment: 5, wantValue: 5, wantLast: 100},
				{eventTime: 90, increment: 5, wantValue: 10, wantLast: 100}, // dt <= 0: no decay
				{eventTime: 100, increment: 5, wantValue: 15, wantLast: 100},
				{eventTime: 105, increment: 5, wantValue: 15, wantLast: 105}, // 20 - 1*5
			},
		},
		{
			name:   "first event sets last update",
			leak:   2,
			volume: 100,
			steps: []step{
				{eventTime: 500, increment: 3, wantValue: 3, wantLast: 500},
				{eventTime: 503, increment: 3, wantValue: 0, wantLast: 503}, // 6 - 2*3
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Record
			for i, s := range tt.steps {
				r.Accumulate(s.increment, tt.leak, s.eventTime)
				if got := r.Value; got != s.wantValue {
					t.Errorf("step %d: value = %v, want %v", i, got, s.wantValue)
				}
				if got := r.LastUpdate; got != s.wantLast {
					t.Errorf("step %d: last update = %v, want %v", i, got, s.wantLast)
				}
				if got := r.Overflow(tt.volume); got != s.wantOverflow {
					t.Errorf("step %d: overflow = %v, want %v", i, got, s.wantOverflow)
				}
			}
		})
	}
}

// TestRecordValueAt: the record's value at a later instant decays by
// leak per second without mutating the record - the Accumulate math
// between events (spec: Dynamic report liveness).
func TestRecordValueAt(t *testing.T) {
	rec := Record{Value: 11000, LastUpdate: 100}
	for _, tt := range []struct {
		name string
		leak float64
		at   int64
		want float64
	}{
		{"no elapsed time holds the value", 1000, 100, 11000},
		{"one second decays by one leak", 1000, 101, 10000},
		{"linear in the gap", 1000, 106, 5000},
		{"floored at zero", 1000, 112, 0},
		{"an earlier instant does not undecay", 1000, 90, 11000},
		{"zero leak never decays", 0, 100000, 11000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.ValueAt(tt.leak, tt.at); got != tt.want {
				t.Errorf("ValueAt(%v, %d) = %v, want %v", tt.leak, tt.at, got, tt.want)
			}
		})
	}
	if rec.Value != 11000 || rec.LastUpdate != 100 {
		t.Errorf("record mutated: %+v", rec)
	}
	if got := (Record{}).ValueAt(1000, 100); got != 0 {
		t.Errorf("empty record ValueAt = %v, want 0", got)
	}
}

// TestRecordUnderflow: the record has underflowed once its value decays
// to volume - dead at value <= volume, the strict mirror of Overflow
// (spec: Dynamic report liveness).
func TestRecordUnderflow(t *testing.T) {
	rec := Record{Value: 11000, LastUpdate: 100}
	for _, tt := range []struct {
		name string
		vol  float64
		at   int64
		want bool
	}{
		{"strictly above is alive", 10000, 100, false},
		{"decayed to the volume underflows", 10000, 101, true},
		{"below the volume underflows", 10000, 102, true},
		{"a higher line underflows sooner", 11000, 100, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Underflow(tt.vol, 1000, tt.at); got != tt.want {
				t.Errorf("Underflow(%v, 1000, %d) = %v, want %v", tt.vol, tt.at, got, tt.want)
			}
		})
	}
}
