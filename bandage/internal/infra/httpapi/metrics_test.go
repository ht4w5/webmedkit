package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// These tests pin the /metrics wire contract (ADR 0014): exposition 0.0.4
// bytes on the wire and nothing else. The seam is the handler constructor
// (New) with a fixed injected snapshot — the same httptest + fake StatsSource
// pattern the bench test uses — so no daemon is needed and the whole surface
// is observable as response bytes.

// metricsFixture is the fixed injected snapshot the golden response is pinned
// against; every value is chosen so each family's rendering is unambiguous. It
// is the whole renderer input — stats and runtime alike — so the golden bytes
// are reproducible from this one value: renderMetrics(metricsFixture).
var metricsFixture = metricsSnapshot{
	stats: Stats{
		StartedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), // Unix 1704164645
		Build: Build{
			Name:      "bandage",
			Version:   "1.2.3",
			Commit:    "abc123",
			BuildDate: "2024-01-02T03:04:05Z",
			Dirty:     false,
			GoVersion: "go1.27.0",
			Platform:  "linux/amd64",
		},
		Pipeline:     Pipeline{Workers: 4, Received: 1000, Parsed: 990, Dropped: 10},
		TrackerCache: TrackerCache{Gets: 800, Sets: 400, Misses: 200, Evictions: 5, Size: 350, EntryCap: 1024},
	},
	runtime: Runtime{
		Version:        "go1.27.0",
		Goroutines:     7,
		Threads:        5,
		AllocBytes:     1234567,
		HeapInuseBytes: 2345678,
		SysBytes:       3456789,
		GCCPUFraction:  0.0125,
	},
}

// fixedStats serves a fixed snapshot: the metrics tests' StatsSource.
type fixedStats struct{ st Stats }

func (f fixedStats) Stats() Stats { return f.st }

// fixedRuntime serves a fixed runtime sample: the metrics tests' RuntimeSource.
type fixedRuntime struct{ rt Runtime }

func (f fixedRuntime) Runtime() Runtime { return f.rt }

// metricsRequest drives one request through New and returns the recorded
// response. The ReportStore is nil: GET /metrics never touches the report API.
func metricsRequest(t *testing.T, snap metricsSnapshot, method string) *httptest.ResponseRecorder {
	t.Helper()
	srv := New(nil, fixedStats{snap.stats}, fixedRuntime{snap.runtime}, func() time.Time { return time.Unix(0, 0) })
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(method, "/metrics", nil))
	return w
}

// metricsGoldenResponse is the canonical GET /metrics body over
// metricsFixture: family order and every byte, newline-terminated lines.
const metricsGoldenResponse = `# HELP bandage_start_time_seconds Seconds since the Unix epoch at which the daemon started.
# TYPE bandage_start_time_seconds gauge
bandage_start_time_seconds 1704164645
# HELP bandage_build_info Build identity of the running binary; the labels carry the identity and the value is always 1.
# TYPE bandage_build_info gauge
bandage_build_info{name="bandage",version="1.2.3",commit="abc123",build_date="2024-01-02T03:04:05Z",dirty="false",go_version="go1.27.0",platform="linux/amd64"} 1
# HELP bandage_pipeline_events_received_total Log lines received from the wire since daemon start.
# TYPE bandage_pipeline_events_received_total counter
bandage_pipeline_events_received_total 1000
# HELP bandage_pipeline_events_parsed_total Log lines parsed into events since daemon start.
# TYPE bandage_pipeline_events_parsed_total counter
bandage_pipeline_events_parsed_total 990
# HELP bandage_pipeline_events_dropped_total Malformed or invalid log lines dropped since daemon start.
# TYPE bandage_pipeline_events_dropped_total counter
bandage_pipeline_events_dropped_total 10
# HELP bandage_pipeline_workers Pipeline workers processing log lines.
# TYPE bandage_pipeline_workers gauge
bandage_pipeline_workers 4
# HELP bandage_tracker_cache_gets_total Tracker cache lookups since daemon start.
# TYPE bandage_tracker_cache_gets_total counter
bandage_tracker_cache_gets_total 800
# HELP bandage_tracker_cache_sets_total Tracker cache writes since daemon start.
# TYPE bandage_tracker_cache_sets_total counter
bandage_tracker_cache_sets_total 400
# HELP bandage_tracker_cache_misses_total Tracker cache lookups that found no entry since daemon start.
# TYPE bandage_tracker_cache_misses_total counter
bandage_tracker_cache_misses_total 200
# HELP bandage_tracker_cache_evictions_total Tracker cache evictions since daemon start.
# TYPE bandage_tracker_cache_evictions_total counter
bandage_tracker_cache_evictions_total 5
# HELP bandage_tracker_cache_entries Tracker cache entries currently held.
# TYPE bandage_tracker_cache_entries gauge
bandage_tracker_cache_entries 350
# HELP bandage_tracker_cache_entry_cap Configured tracker cache entry capacity.
# TYPE bandage_tracker_cache_entry_cap gauge
bandage_tracker_cache_entry_cap 1024
# HELP go_info Information about the Go environment.
# TYPE go_info gauge
go_info{version="go1.27.0"} 1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 7
# HELP go_threads Number of OS threads created.
# TYPE go_threads gauge
go_threads 5
# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 1234567
# HELP go_memstats_heap_inuse_bytes Number of heap bytes that are in use.
# TYPE go_memstats_heap_inuse_bytes gauge
go_memstats_heap_inuse_bytes 2345678
# HELP go_memstats_sys_bytes Number of bytes obtained from system.
# TYPE go_memstats_sys_bytes gauge
go_memstats_sys_bytes 3456789
# HELP go_memstats_gc_cpu_fraction The fraction of this program's available CPU time used by the GC since the program started.
# TYPE go_memstats_gc_cpu_fraction gauge
go_memstats_gc_cpu_fraction 0.0125
`

// TestMetricsGoldenResponse pins the full response byte-exact over the fixed
// injected snapshot — family order, HELP/TYPE text and all — so any change to
// the machine contract is a deliberate act against a golden.
func TestMetricsGoldenResponse(t *testing.T) {
	w := metricsRequest(t, metricsFixture, http.MethodGet)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != metricsGoldenResponse {
		t.Errorf("GET /metrics body =\n%s\nwant\n%s", got, metricsGoldenResponse)
	}
}

// TestMetricsEscapesLabelValues: a nasty commit (quote, backslash, newline)
// is escaped per the exposition spec and cannot corrupt the line framing.
func TestMetricsEscapesLabelValues(t *testing.T) {
	snap := metricsFixture
	snap.stats.Build.Commit = "a\"b\\c\nd"
	w := metricsRequest(t, snap, http.MethodGet)
	want := `bandage_build_info{name="bandage",version="1.2.3",commit="a\"b\\c\nd",build_date="2024-01-02T03:04:05Z",dirty="false",go_version="go1.27.0",platform="linux/amd64"} 1` + "\n"
	if !strings.Contains(w.Body.String(), want) {
		t.Errorf("GET /metrics body lacks escaped line %q, body:\n%s", want, w.Body.String())
	}
	if strings.Contains(w.Body.String(), snap.stats.Build.Commit) {
		t.Errorf("GET /metrics body contains the raw nasty commit %q", snap.stats.Build.Commit)
	}
}

// TestMetricsHelpAndTypeOnEveryFamily: every family carries its # HELP and
// # TYPE lines with the contracted type, so a scraper needs no special cases.
func TestMetricsHelpAndTypeOnEveryFamily(t *testing.T) {
	w := metricsRequest(t, metricsFixture, http.MethodGet)
	body := w.Body.String()
	for _, f := range []struct{ name, kind string }{
		{"bandage_start_time_seconds", "gauge"},
		{"bandage_build_info", "gauge"},
		{"bandage_pipeline_events_received_total", "counter"},
		{"bandage_pipeline_events_parsed_total", "counter"},
		{"bandage_pipeline_events_dropped_total", "counter"},
		{"bandage_pipeline_workers", "gauge"},
		{"bandage_tracker_cache_gets_total", "counter"},
		{"bandage_tracker_cache_sets_total", "counter"},
		{"bandage_tracker_cache_misses_total", "counter"},
		{"bandage_tracker_cache_evictions_total", "counter"},
		{"bandage_tracker_cache_entries", "gauge"},
		{"bandage_tracker_cache_entry_cap", "gauge"},
		{"go_info", "gauge"},
		{"go_goroutines", "gauge"},
		{"go_threads", "gauge"},
		{"go_memstats_alloc_bytes", "gauge"},
		{"go_memstats_heap_inuse_bytes", "gauge"},
		{"go_memstats_sys_bytes", "gauge"},
		{"go_memstats_gc_cpu_fraction", "gauge"},
	} {
		if !strings.Contains(body, "# HELP "+f.name+" ") {
			t.Errorf("body lacks # HELP for %s", f.name)
		}
		if want := "# TYPE " + f.name + " " + f.kind + "\n"; !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}

// TestMetricsContentTypeRoundTrip: the response declares the exposition
// format version so a scraper recognizes the format without guessing.
func TestMetricsContentTypeRoundTrip(t *testing.T) {
	w := metricsRequest(t, metricsFixture, http.MethodGet)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", w.Code)
	}
	const want = "text/plain; version=0.0.4; charset=utf-8"
	if got := w.Header().Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

// TestMetricsRejectsNonGET: the endpoint is read-only; ServeMux's method
// pattern answers anything else with 405 (pinned here).
func TestMetricsRejectsNonGET(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		w := metricsRequest(t, metricsFixture, method)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /metrics = %d, want 405", method, w.Code)
		}
	}
}
