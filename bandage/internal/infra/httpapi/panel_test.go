package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// Web Panel serving tests: the page and its assets are embedded in the
// daemon binary and load from the existing listener; the panel fetches
// data from the machine API only, never an external asset.

// GET / at the exact web root serves the panel page: the tab shell with
// the Statistic and Reports slots, the Reports slot carrying the report
// review table.
func TestWebPanelRootServesPage(t *testing.T) {
	w := apiRequest(t, apiTestServer(nil), http.MethodGet, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	for _, want := range []string{"Statistic", "Reports"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / body lacks the %q tab slot, body:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "placeholder") {
		t.Errorf("GET / body still carries a placeholder instead of the report review table, body:\n%s", body)
	}
}

// The Reports tab lists one row per live Report - rule, target, tracker
// reading, updated time - fetched from the machine API's /api/reports.
// Expired reports never reach the wire (the repository never returns
// them), so the list is exactly what a consumer would fetch.
func TestWebPanelReportsTabListsRowsFromReportsAPI(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	if !strings.Contains(js, "/api/reports") {
		t.Errorf("panel.js does not fetch /api/reports; the Reports tab must list live reports, body:\n%s", js)
	}
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	if !strings.Contains(page, `id="report-rows"`) {
		t.Errorf("GET / body lacks the report review table's row slot, body:\n%s", page)
	}
	for _, want := range []string{"Rule", "Target", "Tracker reading", "Updated"} {
		if !strings.Contains(page, want) {
			t.Errorf("GET / body lacks the %q review-table column, body:\n%s", want, page)
		}
	}
}

// A report row expands to the full report body pretty-printed - reason,
// penalty, sample and expiry are all in the stringified entry - and
// collapses again on a second click. Each row carries its own expand
// state, so expanding one row never disturbs the others.
func TestWebPanelReportRowsExpandToPrettyPrintedBody(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/panel.js").Body.String()
	if !regexp.MustCompile(`JSON\.stringify\(\s*report\w*\s*,\s*null,\s*2\s*\)`).MatchString(js) {
		t.Errorf("panel.js does not pretty-print the report body with JSON.stringify(report, null, 2), body:\n%s", js)
	}
	if !strings.Contains(js, "aria-expanded") {
		t.Errorf("panel.js tracks no per-row expand state; a click must expand and collapse just that row's body, body:\n%s", js)
	}
}

// Manual refresh reloads the report list along with the Statistic, so
// the table always reflects the current report set. An empty report set
// renders a clear empty state, never a blank table.
func TestWebPanelReportsRefreshAndEmptyState(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/panel.js").Body.String()
	if !regexp.MustCompile(`(?s)function refreshAll\(\)\s*\{[^}]*loadStatistic\(\)[^}]*loadReports\(\)`).MatchString(js) {
		t.Errorf("the manual Refresh does not reload the report list along with the Statistic, body:\n%s", js)
	}
	if !strings.Contains(js, "No live reports") {
		t.Errorf("panel.js carries no empty-state message for an empty report set, body:\n%s", js)
	}
}

// Each report row offers a delete button that deletes exactly the
// reviewed Report by its identity - rule and target as the query - via
// DELETE /api/reports and, on completion, refetches the list so the
// deleted Report is gone from the table.
func TestWebPanelReportRowsOfferDeleteByIdentity(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	if !strings.Contains(page, "Delete") {
		t.Errorf("GET / body carries no per-row delete control, body:\n%s", page)
	}
	if !strings.Contains(js, `"/api/reports?rule="`) || !strings.Contains(js, `"&target="`) {
		t.Errorf("panel.js does not address DELETE /api/reports by rule and target filters, body:\n%s", js)
	}
	if !strings.Contains(js, "encodeURIComponent(report.rule)") || !strings.Contains(js, "encodeURIComponent(report.target)") {
		t.Errorf("panel.js does not delete by the reviewed report's (rule, target) identity, body:\n%s", js)
	}
	if !regexp.MustCompile(`method:\s*"DELETE"`).MatchString(js) {
		t.Errorf("panel.js issues no DELETE; a per-row delete must call the reports API's DELETE, body:\n%s", js)
	}
	if !regexp.MustCompile(`(?s)function deleteReport[\s\S]{0,800}?loadReports\(\)`).MatchString(js) {
		t.Errorf("deletion does not refetch the report list; after a delete the Report must be gone from the table, body:\n%s", js)
	}
}

// The delete button's first click is a confirmation step, never an
// immediate delete: only the confirm control issues the DELETE, and
// Cancel deletes nothing. The confirmation copy states that the
// enforcement derived from the report will be withdrawn at the enforcing
// consumer's next sync - naming no specific consumer, since deployments
// may run any ban-rule consumer.
func TestWebPanelDeleteIsTwoStepWithWithdrawalCopy(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	if !regexp.MustCompile(`(?s)function deleteCell[\s\S]{0,600}?showDeleteConfirm`).MatchString(js) {
		t.Errorf("the delete button's first click does not ask for confirmation; delete must be two-step (click, then confirm), body:\n%s", js)
	}
	if !regexp.MustCompile(`(?s)function showDeleteConfirm[\s\S]{0,900}?deleteReport\(`).MatchString(js) {
		t.Errorf("the confirmation step never issues the delete, body:\n%s", js)
	}
	for _, want := range []string{"Confirm", "Cancel"} {
		if !strings.Contains(js, want) {
			t.Errorf("the confirmation step lacks the %q control; cancelling must delete nothing, body:\n%s", want, js)
		}
	}
	for _, want := range []string{"enforcing consumer", "next sync", "withdraw"} {
		if !strings.Contains(js, want) {
			t.Errorf("the delete confirmation copy lacks %q; it must state that the enforcement derived from the report is withdrawn at the enforcing consumer's next sync, body:\n%s", want, js)
		}
	}
	for _, body := range []string{js, page} {
		if strings.Contains(body, "splint") {
			t.Errorf("the panel names a specific consumer (%q); the delete confirmation must name no consumer, body:\n%s", "splint", body)
		}
	}
}

// Every asset the page references resolves from the same listener: each
// href/src points at a local path that serves non-empty bytes.
func TestWebPanelAssetsResolve(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	refs := regexp.MustCompile(`(?:href|src)="([^"]+)"`).FindAllStringSubmatch(page, -1)
	if len(refs) == 0 {
		t.Fatalf("GET / body references no assets, body:\n%s", page)
	}
	for _, m := range refs {
		ref := m[1]
		if strings.HasPrefix(ref, "//") || strings.Contains(ref, ":") {
			t.Errorf("asset reference %q is not a plain local path", ref)
			continue
		}
		// Resolve against the page's base URL like a browser would.
		path := ref
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		w := apiRequest(t, srv, http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, w.Code)
			continue
		}
		if w.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", path)
		}
	}
}

// The panel fetches nothing external: no served asset may reference an
// http(s) URL at all, so the daemon binary alone serves the whole panel.
func TestWebPanelFetchesNoExternalAssets(t *testing.T) {
	srv := apiTestServer(nil)
	for _, path := range []string{"/", "/panel.css", "/panel.js"} {
		body := apiRequest(t, srv, http.MethodGet, path).Body.String()
		for _, scheme := range []string{"http://", "https://"} {
			if strings.Contains(body, scheme) {
				t.Errorf("GET %s body references external URL scheme %q; the panel must fetch no external assets", path, scheme)
			}
		}
	}
}

// The Statistic tab renders the /api/stats snapshot and reloads only on
// manual refresh - the page scripts the machine API and no timer.
func TestWebPanelStatisticTabIsManualAndWiredToStatsAPI(t *testing.T) {
	body := apiRequest(t, apiTestServer(nil), http.MethodGet, "/panel.js").Body.String()
	if !strings.Contains(body, "/api/stats") {
		t.Errorf("panel.js does not fetch /api/stats; the Statistic tab must render the snapshot, body:\n%s", body)
	}
	if strings.Contains(body, "setInterval") {
		t.Errorf("panel.js schedules a timer; refresh must be manual only, body:\n%s", body)
	}
	page := apiRequest(t, apiTestServer(nil), http.MethodGet, "/").Body.String()
	if !strings.Contains(page, `id="refresh"`) {
		t.Errorf("GET / body lacks the manual Refresh control, body:\n%s", page)
	}
}
