// Package domain is the analyzer's domain core: the Event value type flowing through
// enrichment and evaluation, the rule model, and the Report entry that is
// the product of the analyzer.
package domain

import (
	"net/netip"
	"time"
)

// FilesizeUnknown marks an event whose Filesize is unknown - no usable
// size as served. Zero means unknown and nothing else (ADR 0024): empty
// content is indistinguishable from unknown.
const FilesizeUnknown int64 = 0

// ServedFilesize derives the Filesize from a response as served
// (GLOSSARY: Filesize): the Content-Range total on a 206 - its partial
// Content-Length must never masquerade as a file size - and the
// Content-Length otherwise when greater than zero. There is no method
// gating, and an unusable or absent source reads as unknown
// (FilesizeUnknown). The derivation is fixed and total (ADR 0032):
// every response yields exactly one Filesize.
func ServedFilesize(status int, contentLength, contentRangeTotal int64) int64 {
	switch {
	case status == 206 && contentRangeTotal > 0:
		return contentRangeTotal
	case status != 206 && contentLength > 0:
		return contentLength
	}
	return FilesizeUnknown
}

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
	// RemoteAddr is the exact client address behind the report's
	// aggregated Target; it renders in the wire form in text form, the
	// zero Addr as the empty string (netip.Addr's text marshalling).
	RemoteAddr netip.Addr `json:"remote_addr"`
	Method     string     `json:"method"`
	Path       string     `json:"path"`
	Agent      string     `json:"agent"`
	Status     int        `json:"status"`
	// SentBytes renders under its contract key "sent_bytes", the
	// response payload bytes sent.
	SentBytes int64 `json:"sent_bytes"`
	// SizeBytes renders under its contract key "size_bytes", the
	// Filesize: the file's size as served with this response (GLOSSARY:
	// Filesize) - the Content-Range total on a 206, the Content-Length
	// otherwise when greater than zero. It is parse-time state; 0 means
	// unknown (FilesizeUnknown).
	SizeBytes int64 `json:"size_bytes"`
}
