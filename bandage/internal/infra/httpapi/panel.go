package httpapi

import (
	"embed"
	"net/http"
)

// The Web Panel's hand-written HTML, CSS and JS, compiled into the daemon
// binary so deployment stays a single artifact (no build step, no external
// assets fetched). The panel reads data from the machine API client-side
// and adds no data endpoints of its own.

//go:embed web/index.html web/panel.css web/panel.js
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
