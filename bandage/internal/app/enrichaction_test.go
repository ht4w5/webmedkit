package app

import (
	"strings"
	"testing"
)

// Primary seam: the enrich action's load-time contract at the assembled
// daemon - rules YAML in, pointed load errors out (spec: Testing
// Decisions). A refused rules file never starts a daemon; its pointed
// error is the operator's whole outcome.

// TestE2EPointedLoadFailuresNameWhatTheyReject: every pointed load
// failure lands at the assembled-daemon seam (spec: Testing Decisions) -
// rules YAML as the input surface, the pointed load error out of the
// daemon's rules-loading path (loadTestRules - the step every started
// test daemon's rules take before assembly). A refused rules file never
// starts a daemon; its pointed error is the operator's whole outcome,
// and every error names what it rejected - a typo is a load error,
// never a silent default.
func TestE2EPointedLoadFailuresNameWhatTheyReject(t *testing.T) {
	tests := []struct {
		name    string
		rules   string
		wantErr string
	}{
		// An unknown enrichment type names the type - a typo can never
		// silently enrich nothing (story 12).
		{name: "enrich unknown type names the type", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: my-rule\n        actions: [{enrich: {type: fileszie}}]\n", wantErr: `rule "my-rule": action enrich: type: unknown enrichment type "fileszie" (want "filesize")`},

		// Unknown keys in the enrich block fail load naming the key
		// (story 13).
		{name: "enrich unknown key names the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, enabled: true}}]\n", wantErr: `action enrich: unknown key "enabled"`},

		// The counter key is strictly boolean when written - every
		// written non-boolean fails load naming the key (story 11,
		// GLOSSARY: Enrichment counter).
		{name: "enrich quoted counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: \"true\"}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich numeric counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: 1}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich sequence counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: [true]}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich null counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: null}}]\n", wantErr: "action enrich: counter: must be true or false"},
		{name: "enrich empty counter value rejected naming the key", rules: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: }}]\n", wantErr: "action enrich: counter: must be true or false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadTestRules(tt.rules)
			if err == nil {
				t.Fatalf("loadTestRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestE2EEnrichmentBlockFailsThroughStrictDecode: the removed
// top-level enrichment block's shapes get no dedicated check (removed
// rule syntax rides generic refusals) - any presence, even empty or
// null, fails the assembled daemon's rules-loading step through the
// strict decoder's unknown-key refusal (spec: Testing Decisions; the
// pin asserts the generic refusal, with the removed shapes as inputs
// only).
func TestE2EEnrichmentBlockFailsThroughStrictDecode(t *testing.T) {
	tests := []struct {
		name  string
		rules string
	}{
		{name: "enrichment block fails load", rules: "version: 0\nenrichment:\n  filter: 'src 10.0.0.0/8'\nchains:\n  - name: main\n    rules:\n      - id: r\n"},
		{name: "empty enrichment block fails load", rules: "version: 0\nenrichment: {}\nchains:\n  - name: main\n    rules:\n      - id: r\n"},
		{name: "null enrichment block fails load", rules: "version: 0\nenrichment: null\nchains:\n  - name: main\n    rules:\n      - id: r\n"},
		{name: "empty enrichment key fails load", rules: "version: 0\nenrichment:\nchains:\n  - name: main\n    rules:\n      - id: r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadTestRules(tt.rules)
			if err == nil {
				t.Fatal("loadTestRules succeeded, want the strict decoder's unknown-key refusal")
			}
			for _, want := range []string{"rules: decode:", "field enrichment not found"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
