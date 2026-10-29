package main

import (
	"regexp"
	"testing"
)

// progNameRe is the single-label slug shape the report contract puts on
// every label of a reason (docs/reports.md, ADR 0016). bandage composes
// its reason prefix from progName plus a dot (ADR 0017), so the program
// name must be a conforming label itself.
var progNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// TestProgNameIsSlugShaped: the program name feeds the reason prefix
// (ADR 0017), so it must stay slug-shaped or every composed reason stops
// conforming to the report contract's label shape. This guard is the
// first test on the command's run seam; the constraint is pinned here
// rather than validated at startup — progName is a compile-time constant
// and the command's behavior is otherwise untouched.
func TestProgNameIsSlugShaped(t *testing.T) {
	if !progNameRe.MatchString(progName) {
		t.Fatalf("progName = %q is not slug-shaped (want %s): "+
			"it becomes the report reason prefix (ADR 0017), so it must satisfy the label shape of the report contract (docs/reports.md, ADR 0017), where every label of a reason matches %s. "+
			"A malformed name yields non-conforming <prefix>.<code> reasons at runtime; fix the rename, not this test",
			progName, progNameRe.String(), progNameRe.String())
	}
}
