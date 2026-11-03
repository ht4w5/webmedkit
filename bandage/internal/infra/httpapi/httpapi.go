// Package httpapi serves the daemon's read-only HTTP surface: the Web
// Panel's pages (GET /stats and GET /reports, with GET / redirecting to
// the Stats page), GET /api/reports, DELETE /api/reports, GET /ping, GET
// /api/stats and GET /metrics (no URL version, no report writes). The
// machine API lives under /api/ (ADR 0022); /ping and /metrics stay at
// the root as infrastructure endpoints.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/report"
)

// ReportStore is the port the API consumes (report.Repository, narrowed beside
// its consumer); the report service satisfies it, never the repository directly.
type ReportStore interface {
	List(ctx context.Context, q report.Query) ([]domain.Report, error)
	Delete(ctx context.Context, q report.Query) (int, error)
}

// Stats is the /api/stats snapshot: build identity plus cumulative-since-start counters.
type Stats struct {
	// StartedAt is the source's start time; uptime is measured from it.
	StartedAt    time.Time
	Build        Build
	Pipeline     Pipeline
	TrackerCache TrackerCache
	// Rules is the rules block: the counter-enabled rules' Rule
	// counters in rules-file order (GLOSSARY: Rule counter).
	Rules []RuleCounter
}

// Build is the /api/stats build block: the identity of the running binary.
type Build struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Dirty     bool   `json:"dirty"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Pipeline is the /api/stats pipeline block: worker count and line accounting, cumulative.
type Pipeline struct {
	Workers  int   `json:"workers"`
	Received int64 `json:"received"`
	Parsed   int64 `json:"parsed"`
	Dropped  int64 `json:"dropped"`
}

// TrackerCache is the /api/stats tracker cache block: track.Statistics with the entry cap.
type TrackerCache struct {
	Gets      int64 `json:"gets"`
	Sets      int64 `json:"sets"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Size      int64 `json:"size"`
	EntryCap  int64 `json:"entry_cap"`
}

// RuleCounter is one rule's entry in the /api/stats rules block: the
// rule ID and its cumulative counts (GLOSSARY: Rule counter).
type RuleCounter struct {
	ID      string `json:"id"`
	Total   int64  `json:"total"`
	Matched int64  `json:"matched"`
}

// StatsSource is the narrow stats port beside its consumer: one snapshot in.
// The daemon satisfies it structurally.
type StatsSource interface {
	Stats() Stats
}

// One point-in-time Go runtime sample (ADR 0014): the curated subset only,
// read at scrape time by RuntimeSource's adapter, never inside the renderer.
type Runtime struct {
	Version        string  // go_info version label.
	Goroutines     int     // go_goroutines.
	Threads        int     // go_threads.
	AllocBytes     uint64  // go_memstats_alloc_bytes.
	HeapInuseBytes uint64  // go_memstats_heap_inuse_bytes.
	SysBytes       uint64  // go_memstats_sys_bytes.
	GCCPUFraction  float64 // go_memstats_gc_cpu_fraction.
}

// RuntimeSource is the narrow runtime port beside its consumer: one
// point-in-time sample in. A standard-library adapter satisfies it.
type RuntimeSource interface {
	Runtime() Runtime
}

// Server is the HTTP API handler.
type Server struct {
	reports ReportStore
	stats   StatsSource
	runtime RuntimeSource
	now     func() time.Time
	mux     *http.ServeMux
}

func New(reports ReportStore, stats StatsSource, runtime RuntimeSource, now func() time.Time) *Server {
	s := &Server{reports: reports, stats: stats, runtime: runtime, now: now, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /{$}", redirectToStats)
	s.mux.HandleFunc("GET /stats", servePanelAsset("web/stats.html", "text/html; charset=utf-8"))
	s.mux.HandleFunc("GET /reports", servePanelAsset("web/reports.html", "text/html; charset=utf-8"))
	s.mux.HandleFunc("GET /common.css", servePanelAsset("web/common.css", "text/css; charset=utf-8"))
	s.mux.HandleFunc("GET /common.js", servePanelAsset("web/common.js", "text/javascript; charset=utf-8"))
	s.mux.HandleFunc("GET /api/reports", s.handleReports)
	s.mux.HandleFunc("DELETE /api/reports", s.handleDeleteReports)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /ping", s.handlePing)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// document is the version-0 report document.
type document struct {
	Version     int             `json:"version"`
	GeneratedAt string          `json:"generated_at"`
	Reports     []domain.Report `json:"reports"`
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	reports, err := s.reports.List(r.Context(), report.Query{
		Rule:   r.URL.Query().Get("rule"),
		Target: r.URL.Query().Get("target"),
		Now:    now,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if reports == nil {
		reports = []domain.Report{}
	}
	w.Header().Set("Content-Type", "application/json")
	// Canonical v0 serialization (docs/reports.md): two-space indent,
	// Encode's trailing newline, default HTML escaping.
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(document{
		Version:     0,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Reports:     reports,
	})
}

// deleteResponse is the DELETE /api/reports body.
type deleteResponse struct {
	Deleted int `json:"deleted"`
}

// handleDeleteReports deletes reports by filter; ?rule= or ?target= is required
// (no delete-all). Tracker state is untouched: still-abusive Targets report again.
func (s *Server) handleDeleteReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rule, target := q.Get("rule"), q.Get("target")
	if rule == "" && target == "" {
		http.Error(w, "missing rule or target filter", http.StatusBadRequest)
		return
	}
	n, err := s.reports.Delete(r.Context(), report.Query{Rule: rule, Target: target})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(deleteResponse{Deleted: n})
}

func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("pong"))
}

// statsDocument is the rendered /api/stats document (ADR 0006): the snapshot plus
// generation timestamp and uptime; no format version (human-facing, unversioned).
type statsDocument struct {
	GeneratedAt  string        `json:"generated_at"`
	Uptime       string        `json:"uptime"`
	Build        Build         `json:"build"`
	Pipeline     Pipeline      `json:"pipeline"`
	TrackerCache TrackerCache  `json:"tracker_cache"`
	Rules        []RuleCounter `json:"rules"`
}

// handleStats serves the human-facing ops snapshot as indented JSON (ADR 0006):
// build identity and cumulative-since-start counters, not a stable machine contract.
func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	st := s.stats.Stats()
	now := s.now()
	if st.Rules == nil {
		st.Rules = []RuleCounter{}
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(statsDocument{
		GeneratedAt:  now.UTC().Format(time.RFC3339),
		Uptime:       now.Sub(st.StartedAt).Round(time.Second).String(),
		Build:        st.Build,
		Pipeline:     st.Pipeline,
		TrackerCache: st.TrackerCache,
		Rules:        st.Rules,
	})
}
