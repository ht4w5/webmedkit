// Package domain is splint's domain: the version-0 report document it
// consumes, and the ban rules it derives from those reports under an
// exclusion policy. It is a strict consumer of the contract in
// docs/reports.md - the published document is the integration surface.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Document is the wire envelope of the version-0 report document
// (docs/reports.md).
type Document struct {
	Version     int      `json:"version"`
	GeneratedAt string   `json:"generated_at"`
	Reports     []Report `json:"reports"`
}

// Report is one wire report entry (docs/reports.md "Report entry").
// Fields the consumer does not act on still mirror the contract: the
// published document is the integration surface.
type Report struct {
	Rule      string   `json:"rule"`
	Chain     string   `json:"chain"`
	Reason    string   `json:"reason"`
	Target    string   `json:"target"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	ExpiresAt string   `json:"expires_at"`
	Tracker   Tracker  `json:"tracker"`
	Sample    Sample   `json:"sample"`
	Penalty   *Penalty `json:"penalty"`
}

// Tracker is the tracker reading that justified a report.
type Tracker struct {
	Track  string  `json:"track"`
	Value  float64 `json:"value"`
	Leak   float64 `json:"leak"`
	Volume float64 `json:"volume"`
}

// Sample is the most recent event that created a report: the whole
// event the contract's sample carries (docs/reports.md "The sample"),
// mirrored field for field but acted on by nothing - ban-rule
// derivation stays Target and Penalty only.
type Sample struct {
	Time              string `json:"time"`
	RemoteAddr        string `json:"remote_addr"`
	Method            string `json:"method"`
	Path              string `json:"path"`
	Agent             string `json:"agent"`
	Status            int    `json:"status"`
	SentBytes         int64  `json:"sent_bytes"`
	ContentLength     int64  `json:"content_length"`
	ContentRangeTotal int64  `json:"content_range_total"`
	SizeBytes         int64  `json:"size_bytes"`
}

// Penalty is the advisory enforcement stance a report carries
// (docs/reports.md "Penalty"): one action, with its rate exactly when
// the action is a limit.
type Penalty struct {
	Action PenaltyAction `json:"action"`
	Rate   *int64        `json:"rate"`
}

// PenaltyAction is one of the five actions the version-0 contract
// closes over.
type PenaltyAction string

const (
	// PenaltyDrop discards the target's traffic silently.
	PenaltyDrop PenaltyAction = "drop"
	// PenaltyReject refuses the target's traffic with an error signal.
	PenaltyReject PenaltyAction = "reject"
	// PenaltyLimitPktRate throttles the target to Rate packets per second.
	PenaltyLimitPktRate PenaltyAction = "limit-pkt-rate"
	// PenaltyLimitBandwidth throttles the target to Rate response-payload
	// bytes per second.
	PenaltyLimitBandwidth PenaltyAction = "limit-bandwidth"
	// PenaltyLimitReqRate throttles the target to Rate requests per
	// second; packet-layer drivers decline it with a reason (ADR 0012).
	PenaltyLimitReqRate PenaltyAction = "limit-req-rate"
)

func (a PenaltyAction) Known() bool {
	switch a {
	case PenaltyDrop, PenaltyReject, PenaltyLimitPktRate, PenaltyLimitBandwidth, PenaltyLimitReqRate:
		return true
	}
	return false
}

func (a PenaltyAction) IsLimit() bool {
	switch a {
	case PenaltyLimitPktRate, PenaltyLimitBandwidth, PenaltyLimitReqRate:
		return true
	}
	return false
}

// ParseDocument decodes a report document as the published contract,
// ignoring unknown fields (its evolution policy). A missing envelope
// field, unknown version or type mismatch refuses the whole cycle -
// unlike Compile's per-report skips; an empty reports array is valid.
func ParseDocument(data []byte) (Document, error) {
	var envelope struct {
		Version     *int      `json:"version"`
		GeneratedAt *string   `json:"generated_at"`
		Reports     *[]Report `json:"reports"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Document{}, fmt.Errorf("report document: parse: %w", err)
	}
	if envelope.Version == nil {
		return Document{}, errors.New("report document: version: required")
	}
	if envelope.GeneratedAt == nil {
		return Document{}, errors.New("report document: generated_at: required")
	}
	if envelope.Reports == nil {
		return Document{}, errors.New("report document: reports: required")
	}
	if *envelope.Version != 0 {
		return Document{}, fmt.Errorf("report document: version: unknown (got %d, want 0)", *envelope.Version)
	}
	return Document{
		Version:     *envelope.Version,
		GeneratedAt: *envelope.GeneratedAt,
		Reports:     *envelope.Reports,
	}, nil
}
