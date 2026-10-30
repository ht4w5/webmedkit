package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/internal/identity"
)

// Test validates the config and rules files and returns the plan (bandage
// --test); it opens no sockets. Plan contract (ADR 0016, ADR 0017): the
// report carries the code namespaced; the rules file declares it bare.
func Test(configPath, rulesPath string, build identity.Build) (string, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return "", err
	}
	rules, err := LoadRules(rulesPath)
	if err != nil {
		return "", err
	}
	return planText(cfg, rules, build), nil
}

func planText(cfg Config, rules Rules, build identity.Build) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", build.Header())
	fmt.Fprintf(&b, "config: ingest syslog/%s %s %s\n", cfg.Ingest.Parser, cfg.Ingest.Syslog.Network, cfg.Ingest.Syslog.Listen)
	fmt.Fprintf(&b, "config: db %s %s\n", cfg.DB.Driver, cfg.DB.SQLite.Path)
	fmt.Fprintf(&b, "config: api %s\n", cfg.API.Listen)
	fmt.Fprintf(&b, "config: cache max_entries %d\n", cfg.Cache.MaxEntries)
	fmt.Fprintf(&b, "enrichment.filter: %s\n", rules.Enrichment.FilterSource)
	for _, chain := range rules.Chains {
		fmt.Fprintf(&b, "chain %s\n", chain.Name)
		for _, rule := range chain.Rules {
			fmt.Fprintf(&b, "  rule %s\n", rule.ID)
			fmt.Fprintf(&b, "    filter: %s\n", rule.FilterSource)
			if rule.Tracker == nil {
				fmt.Fprintf(&b, "    tracker: none\n")
			} else {
				ttl := rule.Tracker.TTL.String()
				if rule.Tracker.Kind == domain.TrackReplay {
					ttl = "per-event"
				}
				if len(rule.Escalations) > 0 {
					// The ladder's rungs carry every threshold, not the tracker line.
					fmt.Fprintf(&b, "    tracker: %s leak=%s ttl=%s\n",
						rule.Tracker.Kind, fmtFloat(rule.Tracker.Leak), ttl)
				} else {
					fmt.Fprintf(&b, "    tracker: %s leak=%s volume=%s ttl=%s\n",
						rule.Tracker.Kind, fmtFloat(rule.Tracker.Leak), rule.Tracker.VolumeDesc(), ttl)
				}
			}
			for _, action := range rule.Actions {
				fmt.Fprintf(&b, "    action %s\n", actionText(action, build.Name()))
			}
			for _, rung := range rule.Escalations {
				fmt.Fprintf(&b, "    rung at=%s", fmtFloat(rung.At))
				if len(rung.Actions) > 0 {
					parts := make([]string, 0, len(rung.Actions))
					for _, action := range rung.Actions {
						parts = append(parts, actionText(action, build.Name()))
					}
					fmt.Fprintf(&b, " %s", strings.Join(parts, ", "))
				}
				fmt.Fprintln(&b)
			}
		}
	}
	return b.String()
}

// actionText renders one action's plan text (no indentation or newline); the describer cannot fail.
func actionText(a domain.Action, progName string) string {
	var ab strings.Builder
	_ = a.Accept(actionDescriber{b: &ab, progName: progName})
	return ab.String()
}

// actionDescriber is the --test side of Action.Accept, appending one action's text to the plan.
type actionDescriber struct {
	b        *strings.Builder
	progName string // the build's program name: the namespace of the report reason (ADR 0017)
}

func (d actionDescriber) VisitReport(a *domain.ReportAction) error {
	fmt.Fprintf(d.b, "report reason=%s ttl=%s penalty=%s", domain.NamespacedReason(d.progName, a.Reason), a.TTL, a.Penalty.Action)
	if a.Penalty.Rate != nil {
		fmt.Fprintf(d.b, " rate=%d", *a.Penalty.Rate)
	}
	return nil
}

func (d actionDescriber) VisitLog(a *domain.LogAction) error {
	fmt.Fprintf(d.b, "log level=%s message=%q", a.Level, a.Message)
	return nil
}

func (d actionDescriber) VisitGoto(target string) error {
	fmt.Fprintf(d.b, "goto target=%s", target)
	return nil
}

func (d actionDescriber) VisitJump(target string) error {
	fmt.Fprintf(d.b, "jump target=%s", target)
	return nil
}

func (d actionDescriber) VisitStop() error {
	fmt.Fprint(d.b, "stop")
	return nil
}

func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
