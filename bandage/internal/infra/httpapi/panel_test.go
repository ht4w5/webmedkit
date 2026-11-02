package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// Web Panel serving tests: the page and its assets are embedded in the
// daemon binary and load from the existing listener; the panel fetches
// data from the machine API only, never an external asset. The panel JS
// deliberately has no test harness (see the spec's Testing Decisions),
// so every assertion here is about the SERVED surface - the page and
// asset bytes on the wire - never the JS's internal structure; the
// panel's interactive behavior is covered manually per the stories.

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
	// The stub's markup hook only - a legitimate placeholder= input
	// attribute must never trip this guard.
	if strings.Contains(body, `class="placeholder"`) {
		t.Errorf("GET / body still carries the placeholder stub instead of the report review table, body:\n%s", body)
	}
}

// The Reports tab lists one row per live Report - rule, target, tracker
// reading, updated time - fetched from the machine API's /api/reports,
// and an empty report set renders a clear empty state, never a blank
// table. Expired reports never reach the wire (the repository never
// returns them), so the list is exactly what a consumer would fetch.
func TestWebPanelReportsTabListsRowsFromReportsAPI(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	if !strings.Contains(js, "/api/reports") {
		t.Errorf("panel.js does not fetch /api/reports; the Reports tab must list live reports, body:\n%s", js)
	}
	if !strings.Contains(js, "No live reports") {
		t.Errorf("panel.js carries no empty-state copy for an empty report set, body:\n%s", js)
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

// A report row expands to the full report body pretty-printed and
// collapses again without disturbing the other rows - covered manually
// per the stories; at the served level the page advertises the
// click-to-expand affordance on the review table.
func TestWebPanelReportsTabAdvertisesRowExpansion(t *testing.T) {
	page := apiRequest(t, apiTestServer(nil), http.MethodGet, "/").Body.String()
	if !strings.Contains(page, "Click a row for the full report body") {
		t.Errorf("GET / body does not advertise expanding a row to the full report body, body:\n%s", page)
	}
}

// Reload is manual: the page offers the Refresh control, the served JS
// fetches both machine API documents (/api/stats and /api/reports), and
// nothing schedules a timer - there is no polling affordance in the
// served text.
func TestWebPanelRefreshIsManualOverMachineAPI(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	if !strings.Contains(page, `id="refresh"`) {
		t.Errorf("GET / body lacks the manual Refresh control, body:\n%s", page)
	}
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	if !strings.Contains(js, "/api/stats") {
		t.Errorf("panel.js does not fetch /api/stats; the Statistic tab must render the snapshot, body:\n%s", js)
	}
	if !strings.Contains(js, "/api/reports") {
		t.Errorf("panel.js does not fetch /api/reports; Refresh must reload the report list too, body:\n%s", js)
	}
	for _, timer := range []string{"setInterval", "setTimeout"} {
		if strings.Contains(js, timer) {
			t.Errorf("panel.js schedules %s; reload must be manual only, body:\n%s", timer, js)
		}
	}
	if strings.Contains(strings.ToLower(page), "auto-refresh") {
		t.Errorf("GET / body offers an auto-refresh affordance; reload must be manual only, body:\n%s", page)
	}
}

// Each report row offers a delete button that deletes exactly the
// reviewed Report by its identity - rule and target as the query - via
// DELETE /api/reports; after completion the list refetches so the
// deleted Report is gone from the table (covered manually per the
// stories; the DELETE contract itself is pinned at the HTTP seam).
func TestWebPanelReportRowsOfferDeleteByIdentity(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	if !strings.Contains(page, "Delete") {
		t.Errorf("GET / body carries no per-row delete control, body:\n%s", page)
	}
	if !strings.Contains(js, "DELETE") {
		t.Errorf("panel.js issues no DELETE; a per-row delete must call the reports API's DELETE, body:\n%s", js)
	}
	if !strings.Contains(js, `"/api/reports?rule="`) || !strings.Contains(js, `"&target="`) {
		t.Errorf("panel.js does not address DELETE /api/reports by rule and target filters, body:\n%s", js)
	}
}

// The delete button's first click is a confirmation step (covered
// manually per the stories); the served text carries the confirmation
// controls and the copy stating that the enforcement derived from the
// report will be withdrawn at the enforcing consumer's next sync -
// naming no specific consumer, since deployments may run any ban-rule
// consumer.
func TestWebPanelDeleteConfirmCopyAndControls(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/panel.js").Body.String()
	page := apiRequest(t, srv, http.MethodGet, "/").Body.String()
	for _, want := range []string{"Confirm", "Cancel"} {
		if !strings.Contains(js, want) {
			t.Errorf("the served panel JS lacks the %q control; cancelling must delete nothing, body:\n%s", want, js)
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
// href/src points at a local path that serves non-empty bytes with the
// content type its extension promises.
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
		if want, ok := assetContentType(path); ok {
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, want) {
				t.Errorf("GET %s Content-Type = %q, want %s", path, ct, want)
			}
		}
	}
}

// assetContentType is the content type an asset extension promises.
func assetContentType(path string) (string, bool) {
	switch {
	case strings.HasSuffix(path, ".css"):
		return "text/css", true
	case strings.HasSuffix(path, ".js"):
		return "text/javascript", true
	case strings.HasSuffix(path, ".html"):
		return "text/html", true
	}
	return "", false
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

// The filter behaves as a search: the served page states the semantics
// (an unanchored, case-insensitive regular expression over rule, target
// and body), and the served JS carries inline copy for a pattern that
// fails to compile. The filtering behavior itself is covered manually
// per the stories.
func TestWebPanelReportsFilterSemanticsAndInlineError(t *testing.T) {
	page := apiRequest(t, apiTestServer(nil), http.MethodGet, "/").Body.String()
	if !strings.Contains(page, "unanchored, case-insensitive regex over rule, target, and body") {
		t.Errorf("GET / body does not state the filter's semantics (unanchored, case-insensitive regex over rule, target, and body), body:\n%s", page)
	}
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/panel.js").Body.String()
	if !strings.Contains(js, "invalid pattern") {
		t.Errorf("panel.js carries no inline error copy for an invalid pattern, body:\n%s", js)
	}
}
