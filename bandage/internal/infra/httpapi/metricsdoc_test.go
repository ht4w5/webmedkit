package httpapi

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// These tests pin docs/metrics.md — the published metrics contract — to the
// implementation, following the report contract's doc-pinning precedent
// (app/reportdoc_test.go): the doc's canonical example must be the golden
// response byte for byte, and its family table must quote the metricFamilies
// contract exactly. Document and implementation cannot drift.

// metricsDocPath is the published metrics contract, relative to this package.
const metricsDocPath = "../../../../docs/metrics.md"

// metricsDocExample extracts the canonical example from the metrics contract:
// the first fenced ```text block of docs/metrics.md is the example response,
// so the doc stays the single source of truth for operators.
func metricsDocExample(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fenceOpen, fenceEnd := []byte("```text\n"), []byte("\n```")
	i := bytes.Index(data, fenceOpen)
	if i < 0 {
		t.Fatalf("%s: no fenced ```text block (the first one is the canonical example)", path)
	}
	rest := data[i+len(fenceOpen):]
	j := bytes.Index(rest, fenceEnd)
	if j < 0 {
		t.Fatalf("%s: unterminated ```text block", path)
	}
	if j == 0 {
		t.Fatalf("%s: empty ```text block", path)
	}
	return string(rest[:j])
}

// TestMetricsDocumentMatchesDocsExample: the example fenced in docs/metrics.md
// is the golden GET /metrics body over the golden fixture byte for byte, so
// the published contract and the implementation cannot drift. Every
// exposition line is newline-terminated; the fenced example's last line does
// not carry that trailing newline — one trailing byte, cut before comparison
// (the report doc's pin does the same).
func TestMetricsDocumentMatchesDocsExample(t *testing.T) {
	got, found := bytes.CutSuffix([]byte(metricsGoldenResponse), []byte("\n"))
	if !found {
		t.Fatalf("metricsGoldenResponse = %q, want one trailing newline", metricsGoldenResponse)
	}
	want := metricsDocExample(t, metricsDocPath)
	if !bytes.Equal(got, []byte(want)) {
		t.Errorf("docs/metrics.md example does not match the golden response\n--- got (golden) ---\n%s\n--- want (doc) ---\n%s", got, want)
	}
}

// TestMetricsDocumentTableMatchesMetricFamilies: the doc's family table lists
// every family of the metricFamilies contract with its exact type and help
// text (the meaning column is the pinned HELP string), so a rename, a type
// change or a help edit cannot land without the published contract following.
func TestMetricsDocumentTableMatchesMetricFamilies(t *testing.T) {
	data, err := os.ReadFile(metricsDocPath)
	if err != nil {
		t.Fatalf("read %s: %v", metricsDocPath, err)
	}
	doc := string(data)
	for _, f := range metricFamilies {
		row := "| `" + f.name + "` | " + string(f.kind) + " | " + f.help + " |"
		if !strings.Contains(doc, row) {
			t.Errorf("docs/metrics.md lacks family table row %q", row)
		}
	}
}
