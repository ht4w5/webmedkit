package app

import (
	"os"
	"strings"
	"testing"
)

// The release notes are the spec's Further Notes home for the
// terminality behavior-change callout: rules files that relied on a
// report ending the chain must act on it, so the callout cannot go
// missing from the notes; the two new capabilities ride along as the
// notes' other entries.

const releaseNotesPath = "../../../RELEASE-NOTES.md"

func TestReleaseNotesCallOutTheTerminalityChange(t *testing.T) {
	data, err := os.ReadFile(releaseNotesPath)
	if err != nil {
		t.Fatalf("read %s: %v", releaseNotesPath, err)
	}
	notes := string(data)
	for _, want := range []string{
		"no longer end chain traversal", // the behavior change itself
		"add `stop`",                    // what a rules file must do to keep the old traversal
		"remain the terminal",           // goto and stop terminality is unchanged
		"reason_prefix",                 // the new configurable prefix
		"counter: true",                 // the new rule counters
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("%s lacks the callout %q; the release notes must carry it, notes:\n%s", releaseNotesPath, want, notes)
		}
	}
}
