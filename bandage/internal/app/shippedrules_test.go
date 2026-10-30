package app

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/internal/identity"
)

// The shipped rules config at the loader and validated-plan seams: configs/rules.yml
// loads and validates, and the rules-guide fences keep guide, config and plan from drifting.

const (
	shippedRulesPath  = "../../../configs/rules.yml"
	shippedConfigPath = "../../../configs/bandage.yml"
	rulesGuidePath    = "../../../docs/rules.md"
)

func readShipped(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func sectionFence(t *testing.T, doc, heading, marker string) string {
	t.Helper()
	i := strings.Index(doc, heading)
	if i < 0 {
		t.Fatalf("rules guide has no %q section", heading)
	}
	rest := strings.TrimPrefix(doc[i+len(heading):], "\n")
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	for {
		open := strings.Index(rest, "```")
		if open < 0 {
			t.Fatalf("rules guide: no fence containing %q under %q", marker, heading)
		}
		rest = rest[open+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		end := strings.Index(rest, "\n```")
		if end < 0 {
			t.Fatalf("rules guide: unterminated fence under %q", heading)
		}
		if strings.Contains(rest[:end], marker) {
			return rest[:end]
		}
		rest = rest[end+len("\n```"):]
	}
}

func TestShippedRulesLoad(t *testing.T) {
	if _, err := LoadRules(shippedRulesPath); err != nil {
		t.Fatalf("LoadRules(%s) error: %v", shippedRulesPath, err)
	}
}

// The rules guide's plan excerpt is that output verbatim.
func TestShippedRulesPlanShowsEveryRung(t *testing.T) {
	plan, err := Test(shippedConfigPath, shippedRulesPath, identity.For("bandage"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"rung at=50000000 jump target=inspect",
		"rung at=5000000000 report reason=bandage.iso-egress-throttle ttl=24h0m0s penalty=limit-bandwidth rate=1000000",
		"rung at=40000000000 report reason=bandage.iso-egress-ban ttl=168h0m0s penalty=drop",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}

	excerpt := sectionFence(t, readShipped(t, rulesGuidePath), "## Escalation ladders", "rung at=")
	lines := make(map[string]bool)
	for _, line := range strings.Split(plan, "\n") {
		lines[line] = true
	}
	var missing []string
	for _, line := range strings.Split(excerpt, "\n") {
		if line != "" && !lines[line] {
			missing = append(missing, line)
		}
	}
	if len(missing) > 0 {
		t.Errorf("plan is missing the guide's plan-excerpt lines %q:\n%s", missing, plan)
	}
}

func TestShippedRulesCarryTheGuideLadder(t *testing.T) {
	excerpt := sectionFence(t, readShipped(t, rulesGuidePath), "## Escalation ladders", "escalations:")
	if !strings.Contains(readShipped(t, shippedRulesPath), excerpt) {
		t.Errorf("configs/rules.yml does not carry the guide's worked example verbatim:\n%s", excerpt)
	}
}

// The rules guide's dispatcher excerpt is the shipped example's main
// chain verbatim.
func TestShippedRulesCarryTheGuideDispatcher(t *testing.T) {
	excerpt := sectionFence(t, readShipped(t, rulesGuidePath), "## Entry chains and dispatch", "dispatch-download-abuse")
	if !strings.Contains(readShipped(t, shippedRulesPath), excerpt) {
		t.Errorf("configs/rules.yml does not carry the guide's dispatcher excerpt verbatim:\n%s", excerpt)
	}
}

// The shipped example's front door: the main dispatcher chain, pinned to
// the pattern the rules guide and the shipped comment document ("Entry
// chains and dispatch" in docs/rules.md - the rationale lives there, not
// here).
func TestShippedRulesMainDispatchesEveryTopicalChain(t *testing.T) {
	rules, err := LoadRules(shippedRulesPath)
	if err != nil {
		t.Fatalf("LoadRules(%s) error: %v", shippedRulesPath, err)
	}
	// The topical chains, in file order; the handoff target (inspect) is
	// not one of them.
	want := []string{"download-abuse", "api", "iso-egress"}
	var main domain.Chain
	found := false
	for _, chain := range rules.Chains {
		if chain.Name == domain.EntryChain {
			main, found = chain, true
		}
	}
	if !found {
		t.Fatalf("shipped rules have no chain named %q", domain.EntryChain)
	}
	var got []string
	for _, rule := range main.Rules {
		if rule.FilterSource != "any" {
			t.Errorf("dispatcher rule %q: filter = %q, want none (filter-less)", rule.ID, rule.FilterSource)
		}
		if rule.Tracker != nil {
			t.Errorf("dispatcher rule %q: has a tracker, want none", rule.ID)
		}
		if len(rule.Actions) != 1 || rule.Actions[0].Jump == "" {
			t.Errorf("dispatcher rule %q: actions = %v, want one jump", rule.ID, rule.Actions)
			continue
		}
		got = append(got, rule.Actions[0].Jump)
	}
	if !slices.Equal(got, want) {
		t.Errorf("main dispatches %v, want one jump per topical chain %v in file order", got, want)
	}
}
