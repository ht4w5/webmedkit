package app

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/enrichment"
	"github.com/HT4w5/webmedkit/bandage/internal/infra/cache/memory"
)

// enrichmentRunner runs one enrichment kind for an event: the instance
// shape behind the evaluation.Enrichments seam, reporting the run's
// outcomes (GLOSSARY: Enrichment counter).
type enrichmentRunner interface {
	Enrich(ctx context.Context, event *domain.Event) (observed, enriched bool, err error)
}

// enrichmentBuilders is the closed enrichment-kind vocabulary (ADR
// 0028): one builder per kind, producing the kind's single global
// instance. It is the single source both the loader validates enrich
// action types against and daemon assembly builds from, so the
// vocabulary the loader accepts and the instances the daemon serves
// cannot drift.
var enrichmentBuilders = map[string]func(cache *memory.Store) (enrichmentRunner, error){
	"filesize": func(cache *memory.Store) (enrichmentRunner, error) {
		return enrichment.NewFilesizeEnricher(memory.NewFilesize(cache), filesizePeriod, filesizeMinSamples)
	},
}

// enrichmentTypeWant names the closed vocabulary for load errors,
// built from the single source (enrichmentBuilders): (want "filesize").
var enrichmentTypeWant = func() string {
	names := make([]string, 0, len(enrichmentBuilders))
	for name := range enrichmentBuilders {
		names = append(names, fmt.Sprintf("%q", name))
	}
	slices.Sort(names)
	last := len(names) - 1
	if last == 0 {
		return "(want " + names[0] + ")"
	}
	return "(want " + strings.Join(names[:last], ", ") + " or " + names[last] + ")"
}()

// knownEnrichmentType reports whether type names a known enrichment
// kind (enrichmentBuilders).
func knownEnrichmentType(typ string) bool {
	_, ok := enrichmentBuilders[typ]
	return ok
}

// enrichmentKindNames lists the closed vocabulary in sorted order.
func enrichmentKindNames() []string {
	return slices.Sorted(maps.Keys(enrichmentBuilders))
}

// enrichments is the daemon-side enrichment registry: one global
// instance per kind, shared by every enrich action naming it (ADR
// 0028) - the implementation behind the evaluation.Enrichments seam.
// The evaluator invokes it by kind name; the instances and this
// registry are a seam between evaluator and daemon, not a public
// contract.
type enrichments struct {
	byKind map[string]enrichmentRunner
}

// newEnrichments builds one global instance per known kind (the
// builders are the single source, so every known kind is served).
func newEnrichments(cache *memory.Store) (*enrichments, error) {
	byKind := make(map[string]enrichmentRunner, len(enrichmentBuilders))
	for _, kind := range enrichmentKindNames() {
		run, err := enrichmentBuilders[kind](cache)
		if err != nil {
			return nil, err
		}
		byKind[kind] = run
	}
	return &enrichments{byKind: byKind}, nil
}

// Enrich runs the named enrichment for the event (evaluation.Enrichments).
// A kind the loader accepted is always present; an unknown kind is a
// programming error, reported rather than silently enriching nothing.
func (e *enrichments) Enrich(ctx context.Context, kind string, event *domain.Event) (observed, enriched bool, err error) {
	run, ok := e.byKind[kind]
	if !ok {
		return false, false, fmt.Errorf("enrichment: unknown enrichment type %q", kind)
	}
	return run.Enrich(ctx, event)
}
