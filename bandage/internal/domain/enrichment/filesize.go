// Package enrichment derives Event fields from observed traffic: a Filesize
// estimate (GLOSSARY: Filesize) learned per path from observed response sizes.
package enrichment

import (
	"context"
	"errors"
	"fmt"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// FilesizeRecord is one path's file-size estimate state.
type FilesizeRecord struct {
	// SizeEWMA is the smoothed average size in bytes (integer EWMA).
	SizeEWMA int64
	// Samples counts the observed response sizes of the path.
	Samples int
}

// FilesizeCache stores file-size records keyed by path.
type FilesizeCache interface {
	// Update atomically applies fn to the record for path (zero-valued if none
	// exists); fn runs under the store's per-path lock and must not do I/O.
	Update(ctx context.Context, path string, fn func(*FilesizeRecord)) (FilesizeRecord, error)
}

// FilesizeEnricher estimates each path's file size from observed response sizes.
type FilesizeEnricher struct {
	cache  FilesizeCache
	period int64
}

// NewFilesizeEnricher builds an enricher over cache; period is the EWMA
// window (alpha = 1/period, >= 1).
func NewFilesizeEnricher(cache FilesizeCache, period int64) (*FilesizeEnricher, error) {
	if period < 1 {
		return nil, errors.New("enrichment: period must be at least 1")
	}
	return &FilesizeEnricher{
		cache:  cache,
		period: period,
	}, nil
}

// Enrich sets event.SizeBytes from the path's estimate: seeded exactly
// by the path's first observation and refined by EWMA thereafter,
// FilesizeUnknown until that first observation. Only the Sampling rule
// observes.
// observed reports whether the run fed the estimate (an eligible
// observation under the Sampling rule), enriched whether the run left
// a non-default derived value (GLOSSARY: Enrichment counter).
func (fe *FilesizeEnricher) Enrich(ctx context.Context, event *domain.Event) (observed, enriched bool, err error) {
	obs, ok := sizeObservation(event)
	rec, err := fe.cache.Update(ctx, event.Path, func(r *FilesizeRecord) {
		if !ok {
			return // no observation: read the record, observe nothing
		}
		if r.Samples == 0 {
			// Seed with the first observation exactly (no blending).
			r.SizeEWMA = obs
			r.Samples = 1
			return
		}
		r.SizeEWMA = ewma(r.SizeEWMA, obs, fe.period)
		r.Samples++
	})
	if err != nil {
		return false, false, fmt.Errorf("enrichment: update filesize record: %w", err)
	}

	if rec.Samples == 0 {
		// Unknown until the first observation; stale values overwritten.
		event.SizeBytes = domain.FilesizeUnknown
		return ok, false, nil
	}
	event.SizeBytes = rec.SizeEWMA
	return ok, true, nil
}

// sizeObservation reports the event's response size under the Sampling rule
// (GLOSSARY): GET/HEAD 200 + Content-Length, GET 206 + Content-Range total.
func sizeObservation(event *domain.Event) (int64, bool) {
	switch {
	case event.Status == 200 && (event.Method == "GET" || event.Method == "HEAD") && event.ContentLength > 0:
		return event.ContentLength, true
	case event.Status == 206 && event.Method == "GET" && event.ContentRangeTotal > 0:
		return event.ContentRangeTotal, true
	}
	return 0, false
}

// ewma is prev + (observed-prev)/period with truncating division (no floats).
func ewma(prev, observed, period int64) int64 {
	return prev + (observed-prev)/period
}
