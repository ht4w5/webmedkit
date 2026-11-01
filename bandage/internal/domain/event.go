// Package domain is the analyzer's domain core: the Event value type flowing through
// enrichment and evaluation, the rule model, and the Report entry that is
// the product of the analyzer.
package domain

import (
	"net/netip"
	"time"
)

// FilesizeUnknown marks an event whose estimated file size is not known
// yet (enrichment warm-up).
const FilesizeUnknown int64 = 0

// Event is a single parsed access-log record as it flows through
// enrichment and evaluation. A report's Sample carries the Event whole
// (ADR 0021), and the JSON tags below are the report contract's sample
// shape: the sample's wire form is derived from this type (tags plus
// the timestamp renderer in report.go), never a hand-written field
// list, so a field added here lands in every sample without any
// report-side change.
type Event struct {
	// Original fields.
	// Time renders in the wire form as RFC 3339 UTC at second
	// precision (the renderer truncates sub-second).
	Time time.Time `json:"time"`
	// RemoteAddr is parked out of the report contract until the
	// whole-Event fields land (whole-event-sample ticket 02); once
	// un-parked it renders in text form, the zero Addr as the empty
	// string (netip.Addr's text marshalling).
	RemoteAddr netip.Addr `json:"-"`
	// Method is parked out of the report contract until the
	// whole-Event fields land (whole-event-sample ticket 02).
	Method string `json:"-"`
	Path   string `json:"path"`
	Agent  string `json:"agent"`
	Status int    `json:"status"`
	// SentBytes renders under its v0 contract key "sent"; the key
	// becomes sent_bytes with the contract reshape (whole-event-sample
	// ticket 03).
	SentBytes int64 `json:"sent"`
	// ContentLength is the response's Content-Length as the server
	// reported it; 0 means absent or unusable. Parked out of the
	// report contract until the whole-Event fields land
	// (whole-event-sample ticket 02).
	ContentLength int64 `json:"-"`
	// ContentRangeTotal is the file's total size extracted from the
	// response's Content-Range; 0 means absent or unusable. Parked out
	// of the report contract until the whole-Event fields land
	// (whole-event-sample ticket 02).
	ContentRangeTotal int64 `json:"-"`

	// Derived fields, set by enrichment.
	// SizeBytes renders under its v0 contract key "size"; the key
	// becomes size_bytes with the contract reshape (whole-event-sample
	// ticket 03).
	SizeBytes int64 `json:"size"`
}
