package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

// /metrics (ADR 0006's promised endpoint, ADR 0014): Prometheus text
// exposition 0.0.4 hand-rendered — no client library, no live renderer reads.

// The response states its format version rather than negotiating one.
const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// One point-in-time sample: the snapshot /stats serves plus a Runtime sample,
// both read at scrape time by the handler and never by the renderer.
type metricsSnapshot struct {
	stats   Stats
	runtime Runtime
}

type metricLabel struct {
	name  string
	value string
}

type metricKind string

const (
	metricCounter metricKind = "counter"
	metricGauge   metricKind = "gauge"
)

// One row of the machine-contract family table (ADR 0014). Rows render in
// table order — the fixed exposition order — so a new series is one new row.
type metricFamily struct {
	name   string
	kind   metricKind
	help   string
	labels func(s metricsSnapshot) []metricLabel // nil for unlabeled series
	value  func(s metricsSnapshot) float64
}

// The metric contract single-sourced (ADR 0014): name, type, help and
// source per family, in exposition order; go_ rows mirror the Go collector.
var metricFamilies = []metricFamily{
	{
		name: "bandage_start_time_seconds",
		kind: metricGauge,
		help: "Seconds since the Unix epoch at which the daemon started.",
		value: func(s metricsSnapshot) float64 {
			// Whole-second Unix() plus the nanosecond fraction: exact
			// where UnixNano()/1e9 rounds at this magnitude.
			t := s.stats.StartedAt
			return float64(t.Unix()) + float64(t.Nanosecond())/1e9
		},
	},
	{
		name: "bandage_build_info",
		kind: metricGauge,
		help: "Build identity of the running binary; the labels carry the identity and the value is always 1.",
		labels: func(s metricsSnapshot) []metricLabel {
			b := s.stats.Build
			return []metricLabel{
				{"name", b.Name},
				{"version", b.Version},
				{"commit", b.Commit},
				{"build_date", b.BuildDate},
				{"dirty", strconv.FormatBool(b.Dirty)},
				{"go_version", b.GoVersion},
				{"platform", b.Platform},
			}
		},
		value: func(metricsSnapshot) float64 { return 1 },
	},
	{
		name:  "bandage_pipeline_events_received_total",
		kind:  metricCounter,
		help:  "Log lines received from the wire since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.Pipeline.Received) },
	},
	{
		name:  "bandage_pipeline_events_parsed_total",
		kind:  metricCounter,
		help:  "Log lines parsed into events since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.Pipeline.Parsed) },
	},
	{
		name:  "bandage_pipeline_events_dropped_total",
		kind:  metricCounter,
		help:  "Malformed or invalid log lines dropped since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.Pipeline.Dropped) },
	},
	{
		name:  "bandage_pipeline_workers",
		kind:  metricGauge,
		help:  "Pipeline workers processing log lines.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.Pipeline.Workers) },
	},
	{
		name:  "bandage_tracker_cache_gets_total",
		kind:  metricCounter,
		help:  "Tracker cache lookups since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.TrackerCache.Gets) },
	},
	{
		name:  "bandage_tracker_cache_sets_total",
		kind:  metricCounter,
		help:  "Tracker cache writes since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.TrackerCache.Sets) },
	},
	{
		name:  "bandage_tracker_cache_misses_total",
		kind:  metricCounter,
		help:  "Tracker cache lookups that found no entry since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.TrackerCache.Misses) },
	},
	{
		name:  "bandage_tracker_cache_evictions_total",
		kind:  metricCounter,
		help:  "Tracker cache evictions since daemon start.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.TrackerCache.Evictions) },
	},
	{
		name:  "bandage_tracker_cache_entries",
		kind:  metricGauge,
		help:  "Tracker cache entries currently held.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.TrackerCache.Size) },
	},
	{
		name:  "bandage_tracker_cache_entry_cap",
		kind:  metricGauge,
		help:  "Configured tracker cache entry capacity.",
		value: func(s metricsSnapshot) float64 { return float64(s.stats.TrackerCache.EntryCap) },
	},
	{
		name: "go_info",
		kind: metricGauge,
		help: "Information about the Go environment.",
		labels: func(s metricsSnapshot) []metricLabel {
			return []metricLabel{{"version", s.runtime.Version}}
		},
		value: func(metricsSnapshot) float64 { return 1 },
	},
	{
		name:  "go_goroutines",
		kind:  metricGauge,
		help:  "Number of goroutines that currently exist.",
		value: func(s metricsSnapshot) float64 { return float64(s.runtime.Goroutines) },
	},
	{
		name:  "go_threads",
		kind:  metricGauge,
		help:  "Number of OS threads created.",
		value: func(s metricsSnapshot) float64 { return float64(s.runtime.Threads) },
	},
	{
		name:  "go_memstats_alloc_bytes",
		kind:  metricGauge,
		help:  "Number of bytes allocated and still in use.",
		value: func(s metricsSnapshot) float64 { return float64(s.runtime.AllocBytes) },
	},
	{
		name:  "go_memstats_heap_inuse_bytes",
		kind:  metricGauge,
		help:  "Number of heap bytes that are in use.",
		value: func(s metricsSnapshot) float64 { return float64(s.runtime.HeapInuseBytes) },
	},
	{
		name:  "go_memstats_sys_bytes",
		kind:  metricGauge,
		help:  "Number of bytes obtained from system.",
		value: func(s metricsSnapshot) float64 { return float64(s.runtime.SysBytes) },
	},
	{
		name:  "go_memstats_gc_cpu_fraction",
		kind:  metricGauge,
		help:  "The fraction of this program's available CPU time used by the GC since the program started.",
		value: func(s metricsSnapshot) float64 { return s.runtime.GCCPUFraction },
	},
}

// Both sources read at scrape time: one response, one point-in-time snapshot.
// Explicit method check — ServeMux matches HEAD against GET patterns.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", metricsContentType)
	w.Write(renderMetrics(metricsSnapshot{stats: s.stats.Stats(), runtime: s.runtime.Runtime()}))
}

// Pure snapshot-to-text mapping (ADR 0014): exposition 0.0.4 bytes for one
// snapshot, families in table order. Nothing is read here but the snapshot.
func renderMetrics(s metricsSnapshot) []byte {
	var b strings.Builder
	for _, f := range metricFamilies {
		b.WriteString("# HELP ")
		b.WriteString(f.name)
		b.WriteByte(' ')
		b.WriteString(escapeHelp(f.help))
		b.WriteByte('\n')
		b.WriteString("# TYPE ")
		b.WriteString(f.name)
		b.WriteByte(' ')
		b.WriteString(string(f.kind))
		b.WriteByte('\n')
		b.WriteString(f.name)
		if f.labels != nil {
			b.WriteByte('{')
			for i, l := range f.labels(s) {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteString(l.name)
				b.WriteString(`="`)
				b.WriteString(escapeLabelValue(l.value))
				b.WriteByte('"')
			}
			b.WriteByte('}')
		}
		b.WriteByte(' ')
		b.WriteString(formatValue(f.value(s)))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func escapeLabelValue(v string) string {
	return escapeText(v, true)
}

func escapeHelp(v string) string {
	return escapeText(v, false)
}

// Shared exposition escaping (0.0.4): backslash and newline everywhere;
// quote only in label values — it is legal unescaped in HELP text.
func escapeText(v string, escapeQuote bool) string {
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '"' && escapeQuote:
			b.WriteString(`\"`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Plain digits for whole values, so counters stay readable on the wire;
// shortest round-trip form otherwise. Both parse as exposition floats.
func formatValue(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
