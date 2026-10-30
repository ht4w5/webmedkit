package enrichment

import (
	"context"
	"errors"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Enricher is the enrichment path: the Filter gate (GLOSSARY: Filter) over
// the enrichments, holding the single derived-field reset (ADR 0019).
type Enricher struct {
	filter   domain.Matcher
	filesize *FilesizeEnricher
}

// NewEnricher builds the enrichment path: filter gates it wholly
// (enrichment.filter, default any), filesize is the Filesize enrichment.
func NewEnricher(filter domain.Matcher, filesize *FilesizeEnricher) (*Enricher, error) {
	if filter == nil {
		return nil, errors.New("enrichment: filter must not be nil")
	}
	if filesize == nil {
		return nil, errors.New("enrichment: filesize enricher must not be nil")
	}
	return &Enricher{filter: filter, filesize: filesize}, nil
}

// Enrich resets every derived field, then applies the Filter gate wholly -
// unmatched events end with default derived fields, touching no state (ADR 0019).
func (e *Enricher) Enrich(ctx context.Context, event *domain.Event) error {
	resetDerived(event)
	if !e.filter.Match(event) {
		return nil
	}
	return e.filesize.Enrich(ctx, event)
}

// resetDerived resets every derived Event field to its default - the single
// reset reserved for future enrichments, so none can leak stale state (ADR 0019).
func resetDerived(event *domain.Event) {
	event.SizeBytes = domain.FilesizeUnknown
}
