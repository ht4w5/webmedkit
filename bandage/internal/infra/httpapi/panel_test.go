package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// Web Panel serving tests: the Stats and Reports pages and their shared
// assets are embedded in the daemon binary and load from the existing
// listener; the pages fetch data from the machine API only, never an
// external asset. The panel's JavaScript deliberately has no test
// harness (see the spec's Testing Decisions), so every assertion here
// is about the SERVED surface - the page and asset bytes on the wire -
// never the JS's internal behavior; interactive behavior is covered
// manually per the user stories.

// The web root redirects to the Stats page's own address; the view
// pages live at /stats and /reports (ADR 0022 reserved the web root for
// pages; ADR 0025 records the page split).
func TestWebRootRedirectsToStatsPage(t *testing.T) {
	w := apiRequest(t, apiTestServer(nil), http.MethodGet, "/")
	if w.Code != http.StatusFound {
		t.Fatalf("GET / status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/stats" {
		t.Errorf("GET / Location = %q, want /stats", loc)
	}
}

// Both view pages serve as HTML from the daemon's embedded assets.
func TestViewPagesServeHTML(t *testing.T) {
	srv := apiTestServer(nil)
	for _, path := range []string{"/stats", "/reports"} {
		w := apiRequest(t, srv, http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s Content-Type = %q, want text/html", path, ct)
		}
		if w.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", path)
		}
	}
	// The stub's markup hook only - a legitimate placeholder= input
	// attribute must never trip this guard.
	for _, path := range []string{"/stats", "/reports"} {
		if body := apiRequest(t, apiTestServer(nil), http.MethodGet, path).Body.String(); strings.Contains(body, `class="placeholder"`) {
			t.Errorf("GET %s body still carries the placeholder stub instead of the view, body:\n%s", path, body)
		}
	}
}

// Both pages serve the replicated google/pprof web UI header: the brand
// link to the Stats page, the VIEW menu whose dropdown lists Stats and
// Reports with pprof's active-item styling on the current view, the
// regexp search box (placeholder "Search regexp"), the REFRESH menu
// item, and the right-aligned legend slot (ADR 0025).
func TestViewPagesCarryReplicatedHeaderChrome(t *testing.T) {
	srv := apiTestServer(nil)
	for _, tc := range []struct{ path, active string }{
		{"/stats", `<a class="active" href="/stats">Stats</a>`},
		{"/reports", `<a class="active" href="/reports">Reports</a>`},
	} {
		page := apiRequest(t, srv, http.MethodGet, tc.path).Body.String()
		for _, want := range []string{
			`<a href="/stats">bandage</a>`, // brand link to the Stats page
			`id="view"`,                    // the VIEW menu
			`class="submenu"`,              // its dropdown
			`placeholder="Search regexp"`,  // the regexp search box
			`id="refresh"`,                 // the REFRESH menu item
			`id="legend"`,                  // the legend slot
		} {
			if !strings.Contains(page, want) {
				t.Errorf("GET %s body lacks the header's %s, body:\n%s", tc.path, want, page)
			}
		}
		// The VIEW dropdown lists both views and marks the current one
		// with pprof's active-item styling.
		for _, want := range []string{`>Stats</a>`, `>Reports</a>`, tc.active} {
			if !strings.Contains(page, want) {
				t.Errorf("GET %s body lacks the VIEW dropdown's %s, body:\n%s", tc.path, want, page)
			}
		}
	}
}

// Every hint paragraph is gone: the search box's placeholder is the
// panel's only affordance text. None of the old tab shell's hint copy
// survives on either page, and no page explains endpoints or reload
// semantics.
func TestViewPagesCarryNoHintCopy(t *testing.T) {
	srv := apiTestServer(nil)
	for _, path := range []string{"/stats", "/reports"} {
		page := apiRequest(t, srv, http.MethodGet, path).Body.String()
		for _, banned := range []string{
			`class="hint"`,
			"Click a row for the full report body",
			"unanchored, case-insensitive regex over rule, target, and body",
			"Reload with Refresh",
			"cumulative-since-start snapshot, fetched",
			"Every live Report fetched from",
			"/api",
		} {
			if strings.Contains(page, banned) {
				t.Errorf("GET %s body carries hint copy %q; the search placeholder is the only affordance text, body:\n%s", path, banned, page)
			}
		}
	}
}

// Every served asset opens with the short provenance notice: derived
// from google/pprof, Copyright Google Inc., Apache License 2.0 (ADR
// 0025 - the permanent notice obligation on the derived assets).
func TestServedAssetsOpenWithPprofLicenseNotice(t *testing.T) {
	srv := apiTestServer(nil)
	for _, path := range []string{"/stats", "/reports", "/common.css", "/common.js"} {
		body := apiRequest(t, srv, http.MethodGet, path).Body.String()
		if line, _, _ := strings.Cut(body, "\n"); !strings.Contains(line, "derived from google/pprof") {
			t.Errorf("GET %s does not open with the pprof provenance notice, first line %q", path, line)
		}
		for _, want := range []string{"google/pprof", "Copyright Google Inc.", "Apache License 2.0"} {
			if !strings.Contains(body, want) {
				t.Errorf("GET %s body lacks the license notice's %q, body head:\n%s", path, want, head(body, 200))
			}
		}
	}
}

// head returns the first n bytes of s, for compact failure messages.
func head(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// The shared stylesheet carries the replicated google/pprof design
// language (light theme only): pprof's typography and link color, its
// uppercase menu items and dropdown styling, its active-item highlight,
// and its sticky table headers (ADR 0025).
func TestPanelStylesheetCarriesPprofDesignLanguage(t *testing.T) {
	css := apiRequest(t, apiTestServer(nil), http.MethodGet, "/common.css").Body.String()
	for _, want := range []string{
		"Roboto",                    // pprof's typography
		"#2a66d9",                   // pprof's link color
		"text-transform: uppercase", // pprof's uppercase menu items
		"#6b82d6",                   // pprof's active menu item
		"position: sticky",          // pprof's sticky table headers
	} {
		if !strings.Contains(css, want) {
			t.Errorf("common.css lacks the pprof design token %q, css head:\n%s", want, head(css, 200))
		}
	}
	if strings.Contains(css, "prefers-color-scheme") {
		t.Errorf("common.css carries a dark theme; the panel is light theme only, css head:\n%s", head(css, 200))
	}
}

// The Stats page renders the Stats snapshot (the human-facing
// cumulative-since-start document, no format contract - ADR 0006) as
// grouped key/value tables - identity, pipeline, tracker cache -
// fetched from the machine API's /api/stats, instead of a JSON blob.
func TestStatsPageRendersSnapshotAsGroupedKeyValueTables(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/stats").Body.String()
	for _, want := range []string{
		`<h2 class="group-title">identity</h2>`, `id="group-identity"`,
		`<h2 class="group-title">pipeline</h2>`, `id="group-pipeline"`,
		`<h2 class="group-title">tracker cache</h2>`, `id="group-tracker-cache"`,
		`<th>Key</th>`, `<th>Value</th>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("GET /stats body lacks the grouped key/value table slot %s, body:\n%s", want, page)
		}
	}
	if strings.Contains(page, `id="stats"`) {
		t.Errorf("GET /stats body still renders the snapshot as a JSON blob, body:\n%s", page)
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{
		"/api/stats", // the snapshot comes from the machine API
		`"build"`, `"pipeline"`, `"tracker_cache"`, // the snapshot blocks that feed the groups
		"group-identity", "group-pipeline", "group-tracker-cache",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js does not render the snapshot's %s into the grouped key/value tables, body:\n%s", want, js)
		}
	}
}

// The key/value rows are generated per key the daemon reports - the
// row builder iterates the snapshot's entries, never a fixed field
// list - so keys added to the snapshot later (ADR 0006: fields may be
// added) appear as generic extra rows and nothing the daemon reports
// is dropped by the panel. The rendering itself is verified
// interactively per the user stories; this pins the generic row
// builder's served vocabulary.
func TestStatsRowsAreGeneratedPerReportedKey(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{"Object.entries", "kv-row", "kv-key", "kv-value"} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the generic key/value row builder's %s; rows must be generated per reported key, body:\n%s", want, js)
		}
	}
}

// An unknown top-level block renders within its own generic group - a
// generic extra group table named for the block - never flattened as
// dotted rows into the identity table; the known top-level keys
// (generated_at, uptime) and the build block's fields stay in identity
// as now. The rendering itself is verified interactively per the user
// stories; this pins the generic group's served vocabulary.
func TestStatsUnknownBlocksGetTheirOwnGenericGroup(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{
		`["build", "group-identity"]`, // the build block's fields stay in identity as now
		"group-extra-",               // the generic group's row slot, named for the block
		"group-title",                // its heading names the block
		`createElement("table")`,     // its key/value table
		`key + "." + field`,          // an unknown block's generic extra rows keep the block-key prefix
	} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the generic group's %q; an unknown top-level block must render its own group table named for the block, body:\n%s", want, js)
		}
	}
}

// The header's search box narrows the key/value rows on the Stats view
// the way it narrows report rows on Reports: the same unanchored,
// case-insensitive regex semantics, with an invalid pattern called out
// inline instead of silently matching nothing (the narrowing itself is
// covered manually per the stories).
func TestStatsSearchNarrowsKeyValueRows(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/stats").Body.String()
	if !strings.Contains(page, `id="search-error"`) {
		t.Errorf("GET /stats body lacks the inline search-error slot, body:\n%s", page)
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	if !strings.Contains(js, "No keys match") {
		t.Errorf("common.js lacks the Stats search's no-match copy; the search box must narrow the key/value rows, body:\n%s", js)
	}
}

// The right-aligned header legend on the Stats view shows the build
// identity and uptime, so the running binary's identity is on screen
// at a glance (the identity table carries the full snapshot).
func TestStatsLegendShowsBuildIdentityAndUptime(t *testing.T) {
	js := apiRequest(t, apiTestServer(nil), http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{
		`("legend")`,                           // the right-aligned legend slot
		"build.name", "build.version", "build.commit", // the build identity
		" · uptime ", // the uptime copy
	} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the Stats legend's %s; the legend must show build identity and uptime, body:\n%s", want, js)
		}
	}
}

// The review table renders in pprof's table skin: sticky table headers
// right-aligned over the data, and pprof's row-highlight color on the
// hovered report row (ADR 0025).
func TestReviewTableCarriesPprofTableSkin(t *testing.T) {
	css := apiRequest(t, apiTestServer(nil), http.MethodGet, "/common.css").Body.String()
	for _, want := range []string{
		"position: sticky",  // sticky table headers
		"text-align: right", // right-aligned headers and cells
		"report-row:hover",  // the row highlight
		"#ebf5fb",           // pprof's row-highlight color
	} {
		if !strings.Contains(css, want) {
			t.Errorf("common.css lacks the review table's %q skin token, css head:\n%s", want, head(css, 200))
		}
	}
}

// The Reports page carries the report review table - rule, target,
// tracker reading, updated - with its existing affordances: live
// reports fetched from the machine API's /api/reports, click-to-expand
// detail rows, and a per-row Delete that deletes exactly the reviewed
// Report by rule and target via DELETE /api/reports.
func TestReportsPageReviewTableWithDeleteAndDetail(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/reports").Body.String()
	if !strings.Contains(page, `id="report-rows"`) {
		t.Errorf("GET /reports body lacks the review table's row slot, body:\n%s", page)
	}
	for _, want := range []string{"Rule", "Target", "Tracker reading", "Updated", "Delete"} {
		if !strings.Contains(page, want) {
			t.Errorf("GET /reports body lacks the %q review-table column, body:\n%s", want, page)
		}
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{
		"/api/reports", // live reports come from the machine API
		"DELETE",       // per-row delete
		`"/api/reports?rule="`,
		`"&target="`,                     // delete by the report's identity
		"report-detail", "aria-expanded", // click-to-expand detail rows
		"JSON.stringify(report", // the detail carries the full report body (reason, penalty, sample)
	} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the review table's %q affordance, body:\n%s", want, js)
		}
	}
}

// The empty live set renders a clear empty state, never a bare table:
// the page carries the empty-state slot beside the review table, and
// the served JS swaps the table out for the empty state and back (the
// swap itself is verified manually per the spec's user stories).
func TestReportsPageEmptyStateReplacesBareTable(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/reports").Body.String()
	for _, want := range []string{`id="empty-state"`, `id="report-table"`} {
		if !strings.Contains(page, want) {
			t.Errorf("GET /reports body lacks the empty-state surface's %s, body:\n%s", want, page)
		}
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{
		`getElementById("empty-state")`,  // the empty-state slot
		`getElementById("report-table")`, // the table it replaces
		"No live reports",                // the empty-state copy
	} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the empty state's %q; an empty report set must render the empty state, never a bare table, body:\n%s", want, js)
		}
	}
}

// The Reports page's right-aligned legend shows the live report count:
// the header's legend slot is filled from the fetched report list - the
// full live total, not the narrowed view (the fill itself is verified
// manually per the spec's user stories).
func TestReportsPageLegendShowsLiveReportCount(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/reports").Body.String()
	if !strings.Contains(page, `class="legend" id="legend"`) {
		t.Errorf("GET /reports body lacks the header's right-aligned legend slot, body:\n%s", page)
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{
		`getElementById("legend")`, // the legend fill
		`fetchedReports.length`,    // the count comes from the live report list
		"Live reports: ",           // the live report count's copy
	} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the legend's live report count %q; the Reports legend must show the count of live reports, body:\n%s", want, js)
		}
	}
}

// The delete button's first click is a confirmation step; the served
// text carries the confirmation controls and the copy stating that the
// enforcement derived from the report will be withdrawn at the
// enforcing consumer's next sync - naming no specific consumer, since
// deployments may run any ban-rule consumer.
func TestDeleteConfirmCopyAndControls(t *testing.T) {
	srv := apiTestServer(nil)
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{"Confirm", "Cancel"} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the %q control; cancelling must delete nothing, body:\n%s", want, js)
		}
	}
	for _, want := range []string{"enforcing consumer", "next sync", "withdraw"} {
		if !strings.Contains(js, want) {
			t.Errorf("the delete confirmation copy lacks %q; it must state that the enforcement derived from the report is withdrawn at the enforcing consumer's next sync, body:\n%s", want, js)
		}
	}
	for _, path := range []string{"/stats", "/reports", "/common.css", "/common.js"} {
		if body := apiRequest(t, srv, http.MethodGet, path).Body.String(); strings.Contains(body, "splint") {
			t.Errorf("GET %s names a specific consumer (%q); the delete confirmation must name no consumer, body:\n%s", path, "splint", body)
		}
	}
}

// Reload is manual: both pages offer the REFRESH menu item, the served
// JS refetches both views' data from the machine API (/api/stats and
// /api/reports), and nothing schedules a timer - there is no polling
// affordance in the served text.
func TestRefreshIsManualOverMachineAPI(t *testing.T) {
	srv := apiTestServer(nil)
	for _, path := range []string{"/stats", "/reports"} {
		if page := apiRequest(t, srv, http.MethodGet, path).Body.String(); !strings.Contains(page, `id="refresh"`) {
			t.Errorf("GET %s body lacks the REFRESH menu item, body:\n%s", path, page)
		}
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, doc := range []string{"/api/stats", "/api/reports"} {
		if !strings.Contains(js, doc) {
			t.Errorf("common.js does not fetch %s; REFRESH must refetch both views' data, body:\n%s", doc, js)
		}
	}
	for _, timer := range []string{"setInterval", "setTimeout"} {
		if strings.Contains(js, timer) {
			t.Errorf("common.js schedules %s; reload must be manual only, body:\n%s", timer, js)
		}
	}
	for _, path := range []string{"/stats", "/reports"} {
		if page := apiRequest(t, srv, http.MethodGet, path).Body.String(); strings.Contains(strings.ToLower(page), "auto-refresh") {
			t.Errorf("GET %s body offers an auto-refresh affordance; reload must be manual only, body:\n%s", path, page)
		}
	}
}

// The header's regexp search box filters the report review rows; an
// invalid pattern is called out inline instead of silently matching
// nothing (the filtering behavior itself is covered manually per the
// stories).
func TestSearchBoxFiltersWithInlineError(t *testing.T) {
	srv := apiTestServer(nil)
	page := apiRequest(t, srv, http.MethodGet, "/reports").Body.String()
	if !strings.Contains(page, `id="search-error"`) {
		t.Errorf("GET /reports body lacks the inline search-error slot, body:\n%s", page)
	}
	js := apiRequest(t, srv, http.MethodGet, "/common.js").Body.String()
	for _, want := range []string{"new RegExp", "invalid pattern"} {
		if !strings.Contains(js, want) {
			t.Errorf("common.js lacks the search box's %q behavior/copy, body:\n%s", want, js)
		}
	}
}

// Every asset the pages reference resolves from the same listener:
// each href/src points at a local path that serves non-empty bytes with
// the content type its extension promises.
func TestWebPanelAssetsResolve(t *testing.T) {
	srv := apiTestServer(nil)
	refs := regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	for _, path := range []string{"/stats", "/reports"} {
		page := apiRequest(t, srv, http.MethodGet, path).Body.String()
		matches := refs.FindAllStringSubmatch(page, -1)
		if len(matches) == 0 {
			t.Fatalf("GET %s body references no assets, body:\n%s", path, page)
		}
		for _, m := range matches {
			ref := m[1]
			if strings.HasPrefix(ref, "//") || strings.Contains(ref, ":") {
				t.Errorf("asset reference %q is not a plain local path", ref)
				continue
			}
			// Resolve against the page's base URL like a browser would.
			if !strings.HasPrefix(ref, "/") {
				ref = "/" + ref
			}
			w := apiRequest(t, srv, http.MethodGet, ref)
			if w.Code != http.StatusOK {
				t.Errorf("GET %s (from %s) = %d, want 200", ref, path, w.Code)
				continue
			}
			if w.Body.Len() == 0 {
				t.Errorf("GET %s served an empty body", ref)
			}
			if want, ok := assetContentType(ref); ok {
				if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, want) {
					t.Errorf("GET %s Content-Type = %q, want %s", ref, ct, want)
				}
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

// The panel fetches nothing external: no served asset may spell an
// http(s) URL at all, so the daemon binary alone serves the whole panel.
func TestWebPanelFetchesNoExternalAssets(t *testing.T) {
	srv := apiTestServer(nil)
	for _, path := range []string{"/stats", "/reports", "/common.css", "/common.js"} {
		body := apiRequest(t, srv, http.MethodGet, path).Body.String()
		for _, scheme := range []string{"http://", "https://"} {
			if strings.Contains(body, scheme) {
				t.Errorf("GET %s body references external URL scheme %q; the panel must fetch no external assets", path, scheme)
			}
		}
	}
}
