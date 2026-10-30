// Package enrichment derives Event fields from observed traffic: a
// Filesize estimate (GLOSSARY: Filesize) learned per path from observed
// response sizes.
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

// FilesizeEnricher estimates each path's file size from observed response
// sizes.
type FilesizeEnricher struct {
	cache      FilesizeCache
	period     int64
	minSamples int
}

// NewFilesizeEnricher builds an enricher over cache; period is the EWMA window (alpha = 1/period) and minSamples the warm-up gate (both >= 1).
func NewFilesizeEnricher(cache FilesizeCache, period int64, minSamples int) (*FilesizeEnricher, error) {
	if period < 1 {
		return nil, errors.New("enrichment: period must be at least 1")
	}
	if minSamples < 1 {
		return nil, errors.New("enrichment: minSamples must be at least 1")
	}
	return &FilesizeEnricher{
		cache:      cache,
		period:     period,
		minSamples: minSamples,
	}, nil
}

// Enrich sets event.SizeBytes from the path's estimate: the estimate from
// sample minSamples on (the Nth observation included), else FilesizeUnknown.
// The Sampling rule (GLOSSARY) is hard-coded and admits only GET/HEAD 200
// events carrying a Content-Length and GET 206 events carrying a usable
// Content-Range total; ineligible events add no observation but are still
// enriched with the current estimate.
func (fe *FilesizeEnricher) Enrich(ctx context.Context, event *domain.Event) error {
	obs, ok := observation(event)
	rec, err := fe.cache.Update(ctx, event.Path, func(r *FilesizeRecord) {
		if !ok {
			return // ineligible: read the record, observe nothing
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
		return fmt.Errorf("enrichment: update filesize record: %w", err)
	}

	if rec.Samples >= fe.minSamples {
		event.SizeBytes = rec.SizeEWMA
	} else {
		event.SizeBytes = domain.FilesizeUnknown
	}
	return nil
}

// observation reports the event's size observation under the Sampling
// rule (GLOSSARY): GET/HEAD 200 responses carrying a Content-Length
// observe the Content-Length; GET 206 responses carrying a usable
// Content-Range total observe that total and never their
// Content-Length. Transferred bytes, 304 revalidations, length-less
// responses, headerless or "*"-total ranges, 416 and error bodies never
// observe, whatever else admits an event.
func observation(event *domain.Event) (int64, bool) {
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
