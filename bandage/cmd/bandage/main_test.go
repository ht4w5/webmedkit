package main

import (
	"regexp"
	"testing"
)

// progNameRe is the report contract's label shape (docs/reports.md, ADR 0016):
// progName composes the reason prefix (ADR 0017), so it must conform.
var progNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Pinned by test, not startup validation (ADR 0017): progName is a
// compile-time constant, so a bad rename must fail the suite.
func TestProgNameIsSlugShaped(t *testing.T) {
	if !progNameRe.MatchString(progName) {
		t.Fatalf("progName = %q is not slug-shaped (want %s): "+
			"it becomes the report reason prefix (ADR 0017), so it must satisfy the label shape of the report contract (docs/reports.md, ADR 0016), where every label of a reason matches %s. "+
			"A malformed name yields non-conforming <prefix>.<code> reasons at runtime; fix the rename, not this test",
			progName, progNameRe.String(), progNameRe.String())
	}
}
