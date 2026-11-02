package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/report"
)

// API-path tests for the machine API's move under /api/ (ADR 0022): the
// relocated paths serve the unchanged documents, the old root paths are
// gone (no compatibility aliases), and the infrastructure endpoints stay
// at the root.

// stubStore is a ReportStore that returns canned reports and records the
// queries the handlers pass through; the contract under test is the wire,
// never the store.
type stubStore struct {
	reports []domain.Report
	listed  report.Query
	deleted report.Query
	delN    int
}

func (s *stubStore) List(_ context.Context, q report.Query) ([]domain.Report, error) {
	s.listed = q
	return s.reports, nil
}

func (s *stubStore) Delete(_ context.Context, q report.Query) (int, error) {
	s.deleted = q
	return s.delN, nil
}

// apiTestServer is New with the shared fixtures and a fixed clock, so the
// generated_at is exactly "1970-01-01T00:00:00Z".
func apiTestServer(store ReportStore) *Server {
	return New(store, fixedStats{metricsFixture.stats}, fixedRuntime{metricsFixture.runtime}, func() time.Time {
		return time.Unix(0, 0)
	})
}

func apiRequest(t *testing.T, srv *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

// GET /api/reports serves the unchanged version-0 report document.
func TestAPIReportsPathServesReportDocument(t *testing.T) {
	srv := apiTestServer(&stubStore{reports: []domain.Report{{
		Rule:   "ua-trip",
		Target: "203.0.113.7",
	}}})
	w := apiRequest(t, srv, http.MethodGet, "/api/reports")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/reports status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET /api/reports Content-Type = %q, want application/json", ct)
	}
	var doc struct {
		Version     int              `json:"version"`
		GeneratedAt string           `json:"generated_at"`
		Reports     []map[string]any `json:"reports"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("GET /api/reports body %q: %v", w.Body, err)
	}
	if doc.Version != 0 {
		t.Errorf("document version = %d, want 0", doc.Version)
	}
	if doc.GeneratedAt != "1970-01-01T00:00:00Z" {
		t.Errorf("generated_at = %q, want 1970-01-01T00:00:00Z", doc.GeneratedAt)
	}
	if len(doc.Reports) != 1 || doc.Reports[0]["rule"] != "ua-trip" || doc.Reports[0]["target"] != "203.0.113.7" {
		t.Errorf("reports = %+v, want one ua-trip/203.0.113.7 entry", doc.Reports)
	}
}

// GET /api/stats serves the unchanged Statistic snapshot.
func TestAPIStatsPathServesStatistic(t *testing.T) {
	srv := apiTestServer(nil)
	w := apiRequest(t, srv, http.MethodGet, "/api/stats")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stats status = %d, want 200", w.Code)
	}
	var doc statsDocument
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("GET /api/stats body %q: %v", w.Body, err)
	}
	want := metricsFixture.stats
	if doc.GeneratedAt != "1970-01-01T00:00:00Z" {
		t.Errorf("generated_at = %q, want 1970-01-01T00:00:00Z", doc.GeneratedAt)
	}
	if doc.Build != want.Build || doc.Pipeline != want.Pipeline || doc.TrackerCache != want.TrackerCache {
		t.Errorf("snapshot = %+v, want build/pipeline/tracker_cache of %+v", doc, want)
	}
}

// DELETE /api/reports removes reports by (rule, target) as before.
func TestAPIReportsDeleteRemovesByRuleAndTarget(t *testing.T) {
	store := &stubStore{delN: 1}
	srv := apiTestServer(store)
	w := apiRequest(t, srv, http.MethodDelete, "/api/reports?rule=ua-trip&target=203.0.113.7")
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /api/reports status = %d, want 200 (body %q)", w.Code, w.Body)
	}
	var out deleteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("DELETE /api/reports body %q: %v", w.Body, err)
	}
	if out.Deleted != 1 {
		t.Errorf("deleted = %d, want 1", out.Deleted)
	}
	if store.deleted.Rule != "ua-trip" || store.deleted.Target != "203.0.113.7" {
		t.Errorf("delete query = %+v, want rule=ua-trip target=203.0.113.7", store.deleted)
	}

	// Missing both filters stays a 400 (unchanged DELETE semantics).
	w = apiRequest(t, srv, http.MethodDelete, "/api/reports")
	if w.Code != http.StatusBadRequest {
		t.Errorf("DELETE /api/reports without filters = %d, want 400", w.Code)
	}
}

// The old root paths are gone: no compatibility aliases (ADR 0022).
func TestOldRootAPIPathsReturn404(t *testing.T) {
	srv := apiTestServer(&stubStore{})
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/reports"},
		{http.MethodDelete, "/reports?rule=ua-trip"},
		{http.MethodGet, "/stats"},
	} {
		if w := apiRequest(t, srv, tc.method, tc.target); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.target, w.Code)
		}
	}
}

// The infrastructure endpoints stay at the root: liveness and scrape.
func TestInfrastructureEndpointsStayAtRoot(t *testing.T) {
	srv := apiTestServer(nil)
	w := apiRequest(t, srv, http.MethodGet, "/ping")
	if w.Code != http.StatusOK || w.Body.String() != "pong" {
		t.Errorf("GET /ping = %d %q, want 200 %q", w.Code, w.Body, "pong")
	}
	w = apiRequest(t, srv, http.MethodGet, "/metrics")
	if w.Code != http.StatusOK {
		t.Errorf("GET /metrics = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("GET /metrics Content-Type = %q, want the exposition content type", ct)
	}
}
