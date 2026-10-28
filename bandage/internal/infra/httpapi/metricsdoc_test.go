package httpapi

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Pins docs/metrics.md — the published metrics contract — to the
// implementation, following the report doc's precedent (app/reportdoc_test.go).

// Resolved relative to this package's directory.
const metricsDocPath = "../../../../docs/metrics.md"

// The doc's first fenced ```text block is the canonical example response.
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

// The fenced example is the golden body byte for byte, minus the trailing
// newline the fence cannot carry (cut before comparison).
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

// The table's meaning column is the pinned HELP string; rows must match
// metricFamilies exactly.
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
