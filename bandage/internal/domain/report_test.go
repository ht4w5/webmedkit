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
		// The whole Event is the sample (ADR 0021): every field set
		// here renders in the document, the content observations under
		// the zero-means-absent convention (ticket 02).
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
		"sample": {"time": "2025-10-17T18:29:40Z", "remote_addr": "203.0.113.7", "method": "GET", "path": "/releases/24.04.iso", "agent": "Wget/1.21", "status": 200, "sent_bytes": 123456, "content_length": 123456, "content_range_total": 1234567890, "size_bytes": 1234567890},
		"penalty": {"action": "limit-bandwidth", "rate": 1000000}
	}`
	if compact(string(got)) != compact(want) {
		t.Errorf("Marshal = %s\nwant %s", got, compact(want))
	}
}

// TestReportMarshalJSONDynamicEntry pins the dynamic entry shape (spec:
// Report document): "dynamic": true appends after "penalty" and
// "expires_at" renders as null - the second pinned example beside the
// canonical one, whose bytes must not move.
func TestReportMarshalJSONDynamicEntry(t *testing.T) {
	rec := Report{
		Rule:      "sustained-iso-egress",
		Chain:     "download-abuse",
		Reason:    "iso-egress-overflow",
		Target:    "203.0.113.0/24",
		CreatedAt: time.Date(2025, 10, 17, 10, 2, 11, 0, time.UTC),
		UpdatedAt: time.Date(2025, 10, 17, 18, 29, 40, 500, time.UTC), // sub-second truncated in document
		ExpiresAt: time.Time{},                                        // dynamic: no scheduled withdrawal
		Tracker: TrackerReading{
			Track:  "bandwidth",
			Value:  102400000,
			Leak:   1000000,
			Volume: 100000000,
		},
		Sample: Event{
			Time:       time.Date(2025, 10, 17, 18, 29, 40, 500000000, time.UTC), // sub-second truncated in document
			RemoteAddr: netip.MustParseAddr("203.0.113.7"),
			Method:     "GET",
			Path:       "/releases/24.04.iso",
			Status:     200,
			SentBytes:  123456,
		},
		Penalty: &Penalty{Action: PenaltyLimitBandwidth, Rate: int64Ptr(1000000)},
		Dynamic: true,
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
		"expires_at": null,
		"tracker": {"track": "bandwidth", "value": 102400000, "leak": 1000000, "volume": 100000000},
		"sample": {"time": "2025-10-17T18:29:40Z", "remote_addr": "203.0.113.7", "method": "GET", "path": "/releases/24.04.iso", "agent": "", "status": 200, "sent_bytes": 123456, "content_length": 0, "content_range_total": 0, "size_bytes": 0},
		"penalty": {"action": "limit-bandwidth", "rate": 1000000},
		"dynamic": true
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
	for _, tt := range []struct {
		name string
		orig Event
		want string
	}{
		{
			name: "a fully populated sample carries the whole Event",
			orig: Event{
				Time:              time.Date(2025, 10, 17, 18, 29, 40, 500000000, time.UTC), // truncated to the second on the wire
				RemoteAddr:        netip.MustParseAddr("203.0.113.7"),
				Method:            "GET",
				Path:              "/releases/24.04.iso",
				Agent:             "Wget/1.21",
				Status:            206,
				SentBytes:         6000,
				ContentLength:     6000,
				ContentRangeTotal: 3000000,
				SizeBytes:         3000000,
			},
			want: `{"time":"2025-10-17T18:29:40Z","remote_addr":"203.0.113.7","method":"GET","path":"/releases/24.04.iso","agent":"Wget/1.21","status":206,"sent_bytes":6000,"content_length":6000,"content_range_total":3000000,"size_bytes":3000000}`,
		},
		{
			name: "absent observations render as zeroes",
			orig: Event{
				Time:      time.Date(2025, 10, 17, 18, 29, 40, 500000000, time.UTC),
				Path:      "/releases/24.04.iso",
				Agent:     "Wget/1.21",
				Status:    200,
				SentBytes: 123456,
				SizeBytes: FilesizeUnknown, // absent: zero on the wire and back
			},
			want: `{"time":"2025-10-17T18:29:40Z","remote_addr":"","method":"","path":"/releases/24.04.iso","agent":"Wget/1.21","status":200,"sent_bytes":123456,"content_length":0,"content_range_total":0,"size_bytes":0}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.orig)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if compact(string(b)) != compact(tt.want) {
				t.Errorf("Marshal = %s\nwant %s", b, compact(tt.want))
			}
			var back Event
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			wantBack := tt.orig
			wantBack.Time = time.Date(2025, 10, 17, 18, 29, 40, 0, time.UTC)
			checkSample(t, back, wantBack)
		})
	}
}

// TestSampleUnmarshalAbsentTime pins the absent-field convention at the
// timestamp: a missing or empty "time" reads as the zero time (absent
// sample fields read as zero - zero means absent), while a present
// value still parses at the wire's second precision.
func TestSampleUnmarshalAbsentTime(t *testing.T) {
	for _, tt := range []struct {
		name    string
		in      string
		want    Event
		wantErr bool
	}{
		{
			name: "a missing time member reads as the zero time",
			in:   `{"path":"/releases/24.04.iso","status":200}`,
			want: Event{Path: "/releases/24.04.iso", Status: 200},
		},
		{
			name: "an empty time reads as the zero time",
			in:   `{"time":"","path":"/releases/24.04.iso"}`,
			want: Event{Path: "/releases/24.04.iso"},
		},
		{
			name: "a present time parses at second precision",
			in:   `{"time":"2025-10-17T18:29:40Z"}`,
			want: Event{Time: time.Date(2025, 10, 17, 18, 29, 40, 0, time.UTC)},
		},
		{
			name:    "a malformed time still errors",
			in:      `{"time":"yesterday"}`,
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got Event
			err := json.Unmarshal([]byte(tt.in), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) = %+v, want an encoding error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", tt.in, err)
			}
			checkSample(t, got, tt.want)
		})
	}
}

// checkSample reports every sample field that differs from want as
// "sample <field> = ..., want ..." - the field-by-field comparison
// behind the sample assertions.
func checkSample(t *testing.T, got, want Event) {
	t.Helper()
	if !got.Time.Equal(want.Time) {
		t.Errorf("sample time = %v, want %v", got.Time, want.Time)
	}
	for _, f := range []struct {
		name      string
		got, want any
	}{
		{"remote_addr", got.RemoteAddr, want.RemoteAddr},
		{"method", got.Method, want.Method},
		{"path", got.Path, want.Path},
		{"agent", got.Agent, want.Agent},
		{"status", got.Status, want.Status},
		{"sent_bytes", got.SentBytes, want.SentBytes},
		{"content_length", got.ContentLength, want.ContentLength},
		{"content_range_total", got.ContentRangeTotal, want.ContentRangeTotal},
		{"size_bytes", got.SizeBytes, want.SizeBytes},
	} {
		if f.got != f.want {
			t.Errorf("sample %s = %v, want %v", f.name, f.got, f.want)
		}
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
