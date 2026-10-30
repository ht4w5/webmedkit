package enrichment

import (
	"context"
	"errors"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// Enricher is the enrichment path: the Filter gate (GLOSSARY) over the
// enrichments, holding the single derived-field reset. The Filter decides
// only whether the path considers an event at all - what feeds the
// Filesize estimate is the Sampling rule, fixed in code and stricter than
// any Filter.
type Enricher struct {
	filter   domain.Matcher
	filesize *FilesizeEnricher
}

// NewEnricher builds the enrichment path: filter gates it wholly
// (enrichment.filter, the rule filter language, default any), filesize
// is the Filesize enrichment.
func NewEnricher(filter domain.Matcher, filesize *FilesizeEnricher) (*Enricher, error) {
	if filter == nil {
		return nil, errors.New("enrichment: filter must not be nil")
	}
	if filesize == nil {
		return nil, errors.New("enrichment: filesize enricher must not be nil")
	}
	return &Enricher{filter: filter, filesize: filesize}, nil
}

// Enrich resets every derived field to its default, then applies the
// Filter gate wholly (spec Implementation Decision 2): unmatched events
// end here with default derived fields and touch no enrichment state -
// no observation, no cache update. Matched events run every enrichment;
// ineligible ones are enriched with the current estimate and observe
// nothing.
func (e *Enricher) Enrich(ctx context.Context, event *domain.Event) error {
	resetDerived(event)
	if !e.filter.Match(event) {
		return nil
	}
	return e.filesize.Enrich(ctx, event)
}

// resetDerived resets every derived Event field to its default - the
// single reset reserved for future enrichments, so unmatched events can
// never leak stale or half-computed derived state and every future
// enrichment inherits the gate. Derived fields: SizeBytes.
func resetDerived(event *domain.Event) {
	event.SizeBytes = domain.FilesizeUnknown
}
