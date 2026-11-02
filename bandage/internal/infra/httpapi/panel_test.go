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

// The Reports tab carries one filter box above the review table plus a
// slot for an inline pattern error. The filter narrows the fetched
// report list client-side - a panel convenience over stored Reports,
// deliberately independent of the Filter DSL's regex matchers, which
// evaluate Events during rule evaluation.
func TestWebPanelReportsFilterBox(t *testing.T) {
	page := apiRequest(t, apiTestServer(nil), http.MethodGet, "/").Body.String()
	for _, want := range []string{`id="report-filter"`, `id="report-filter-error"`} {
		if !strings.Contains(page, want) {
			t.Errorf("GET / body lacks the report filter's %s, body:\n%s", want, page)
		}
	}
	reports := strings.Index(page, `id="panel-reports"`)
	filter := strings.Index(page, `id="report-filter"`)
	if reports >= 0 && filter >= 0 && filter < reports {
		t.Errorf("the report filter must sit on the Reports tab, body:\n%s", page)
	}
}

// The filter behaves as a search: the typed pattern compiles as an
// unanchored, case-insensitive regular expression matched across the
// report's rule, target, and stringified body. Typing filters the
// visible rows immediately (the input event re-renders the table), and
// the narrowed list re-renders through the same row builder that
// carries the expand toggle, so expansion keeps working on the
// filtered rows.
func TestWebPanelReportsFilterMatchesRuleTargetBody(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/panel.js").Body.String()
	if !regexp.MustCompile(`new RegExp\(\s*\w+\s*,\s*"i"\s*\)`).MatchString(js) {
		t.Errorf("panel.js does not compile the typed pattern with new RegExp(pattern, \"i\"); the filter must be an unanchored, case-insensitive search, body:\n%s", js)
	}
	if strings.Contains(js, `"^" +`) {
		t.Errorf("panel.js anchors the filter pattern; matching must be unanchored, body:\n%s", js)
	}
	for _, want := range []string{"report.rule", "report.target", "JSON.stringify(report)"} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js does not match the filter across rule, target, and the stringified body (%s missing), body:\n%s", want, js)
		}
	}
	if !strings.Contains(js, `addEventListener("input"`) {
		t.Errorf("panel.js does not filter on typing; the input event must filter the visible rows immediately, body:\n%s", js)
	}
	if strings.Contains(js, "style.display") {
		t.Errorf("panel.js filters by hiding rows in place; the narrowed list must re-render through renderReports so the filtered rows stay expandable, body:\n%s", js)
	}
}

// An invalid pattern is reported inline while the full list stays
// visible - a typo never blanks the view - and clearing the box
// restores the full list. Both fallbacks re-render the full fetched
// list through the same table.
func TestWebPanelReportsFilterInvalidPatternKeepsFullList(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/panel.js").Body.String()
	if !regexp.MustCompile(`(?s)try\s*\{[^}]*new RegExp\([^}]*\}\s*catch`).MatchString(js) {
		t.Errorf("panel.js does not guard the pattern compile with try/catch; an invalid pattern must be caught, never thrown, body:\n%s", js)
	}
	if !strings.Contains(js, "report-filter-error") {
		t.Errorf("panel.js never writes to the inline error slot; an invalid pattern must be reported inline, body:\n%s", js)
	}
	if !strings.Contains(js, "renderReports(fetchedReports)") {
		t.Errorf("panel.js has no full-list fallback render; an invalid pattern must leave the full list visible and clearing the box must restore it, body:\n%s", js)
	}
}
