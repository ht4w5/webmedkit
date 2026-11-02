package httpapi

import (
	"embed"
	"net/http"
)

// The Web Panel's hand-written HTML, CSS and JS, compiled into the daemon
// binary so deployment stays a single artifact (no build step, no external
// assets fetched). The pages read data from the machine API client-side
// and add no data endpoints of their own.

//go:embed web/stats.html web/reports.html web/common.css web/common.js
var panelAssets embed.FS

// servePanelAsset serves one embedded asset with an explicit content type.
func servePanelAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		b, err := panelAssets.ReadFile(name)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(b)
	}
}

// redirectToStats redirects the web root to the Stats page's own address
// (ADR 0025: one bookmarks-able page per view).
func redirectToStats(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/stats", http.StatusFound)
}
