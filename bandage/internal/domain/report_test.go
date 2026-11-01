package domain

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// TestReportMarshalJSON checks the document shape: RFC 3339 UTC
// timestamps at second precision, keys in document order with the
// nested penalty last.
func TestReportMarshalJSON(t *testing.T) {
	rec := Report{
		Rule:      "sustained-iso-egress",
		Chain:     "download-abuse",
		Reason:    "iso-egress-overflow",
		Target:    "203.0.113.0/24",
		CreatedAt: time.Date(2025, 10, 17, 10, 2, 11, 0, time.UTC),
		UpdatedAt: time.Date(2025, 10, 17, 18, 29, 40, 500, time.UTC), // sub-second truncated in document
		ExpiresAt: time.Date(2025, 10, 24, 18, 29, 40, 0, time.UTC),
		Tracker: TrackerReading{
			Track:  "bandwidth",
			Value:  102400000,
			Leak:   1000000,
			Volume: 100000000,
		},
		// The whole Event is the sample (ADR 0021): the parked fields
		// are set here and must stay out of the document (they enter
		// the contract with the whole-Event fields; ticket 02).
		Sample: Event{
			Time:              time.Date(2025, 10, 17, 18, 29, 40, 500000000, time.UTC), // sub-second truncated in document
			RemoteAddr:        netip.MustParseAddr("203.0.113.7"),
			Method:            "GET",
			Path:              "/releases/24.04.iso",
			Agent:             "Wget/1.21",
			Status:            200,
			SentBytes:         123456,
			ContentLength:     123456,
			ContentRangeTotal: 1234567890,
			SizeBytes:         1234567890,
		},
		Penalty: &Penalty{Action: PenaltyLimitBandwidth, Rate: int64Ptr(1000000)},
	}
	got, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{
		"rule": "sustained-iso-egress",
		"chain": "download-abuse",
		"reason": "iso-egress-overflow",
		"target": "203.0.113.0/24",
		"created_at": "2025-10-17T10:02:11Z",
		"updated_at": "2025-10-17T18:29:40Z",
		"expires_at": "2025-10-24T18:29:40Z",
		"tracker": {"track": "bandwidth", "value": 102400000, "leak": 1000000, "volume": 100000000},
		"sample": {"time": "2025-10-17T18:29:40Z", "path": "/releases/24.04.iso", "agent": "Wget/1.21", "status": 200, "sent": 123456, "size": 1234567890},
		"penalty": {"action": "limit-bandwidth", "rate": 1000000}
	}`
	if compact(string(got)) != compact(want) {
		t.Errorf("Marshal = %s\nwant %s", got, compact(want))
	}
}

// TestSampleRoundTrip checks the sample's JSON form - the one wire
// form behind the served document and the persisted body (the sqlite
// adapter round-trips the sample through it): the contract's keys come
// from the Event's field set and tags, timestamps render RFC 3339 UTC
// at second precision with sub-second truncated, and absent values
// round-trip as zero (zero means absent).
func TestSampleRoundTrip(t *testing.T) {
	orig := Event{
		Time:      time.Date(2025, 10, 17, 18, 29, 40, 500000000, time.UTC), // truncated to the second on the wire
		Path:      "/releases/24.04.iso",
		Agent:     "Wget/1.21",
		Status:    200,
		SentBytes: 123456,
		SizeBytes: FilesizeUnknown, // absent: zero on the wire and back
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"time":"2025-10-17T18:29:40Z","path":"/releases/24.04.iso","agent":"Wget/1.21","status":200,"sent":123456,"size":0}`
	if compact(string(b)) != compact(want) {
		t.Errorf("Marshal = %s\nwant %s", b, compact(want))
	}
	var back Event
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	wantBack := orig
	wantBack.Time = time.Date(2025, 10, 17, 18, 29, 40, 0, time.UTC)
	if !back.Time.Equal(wantBack.Time) || back.Path != wantBack.Path || back.Agent != wantBack.Agent ||
		back.Status != wantBack.Status || back.SentBytes != wantBack.SentBytes || back.SizeBytes != wantBack.SizeBytes {
		t.Errorf("round trip = %+v, want %+v", back, wantBack)
	}
}

func compact(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// int64Ptr returns a pointer to v (a limit penalty's rate).
func int64Ptr(v int64) *int64 { return &v }

// TestPenaltyMarshalIncompleteLimits pins the strict side of the wire
// contract: a limit penalty without a valid rate must never serialize
// (an incomplete recommendation can never reach a consumer), while the
// minimum valid rate does.
func TestPenaltyMarshalIncompleteLimits(t *testing.T) {
	for _, tt := range []struct {
		name    string
		penalty Penalty
		want    string // expected fragment, "" when marshaling must fail
	}{
		{"a limit without a rate fails", Penalty{Action: PenaltyLimitPktRate}, ""},
		{"a limit with a zero rate fails", Penalty{Action: PenaltyLimitBandwidth, Rate: int64Ptr(0)}, ""},
		{"a limit with a negative rate fails", Penalty{Action: PenaltyLimitReqRate, Rate: int64Ptr(-1)}, ""},
		{"a limit with the minimum rate marshals", Penalty{Action: PenaltyLimitPktRate, Rate: int64Ptr(1)}, `"action":"limit-pkt-rate","rate":1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.penalty)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("Marshal = %s, want an encoding error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if !strings.Contains(string(got), tt.want) {
				t.Errorf("Marshal = %s, want it to contain %s", got, tt.want)
			}
		})
	}
}

// TestPenaltyWireShape pins the nested penalty object: action always,
// rate exactly for a limit action (integer, never 0), and the penalty
// key itself omitted only when the report carries none.
func TestPenaltyWireShape(t *testing.T) {
	for _, tt := range []struct {
		name    string
		penalty *Penalty
		want    string // expected "penalty" fragment, "" when the key is omitted
	}{
		{"a limit carries its rate", &Penalty{Action: PenaltyLimitPktRate, Rate: int64Ptr(200)}, `"penalty":{"action":"limit-pkt-rate","rate":200}`},
		{"drop carries no rate", &Penalty{Action: PenaltyDrop}, `"penalty":{"action":"drop"}`},
		{"reject carries no rate even when one is set", &Penalty{Action: PenaltyReject, Rate: int64Ptr(200)}, `"penalty":{"action":"reject"}`},
		{"no penalty is omitted", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := Report{Rule: "r", Penalty: tt.penalty}
			got, err := json.Marshal(rec)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if tt.want == "" {
				if strings.Contains(string(got), `"penalty"`) {
					t.Errorf("Marshal = %s, want no penalty key", got)
				}
				return
			}
			if !strings.Contains(string(got), tt.want) {
				t.Errorf("Marshal = %s, want it to contain %s", got, tt.want)
			}
		})
	}
}
