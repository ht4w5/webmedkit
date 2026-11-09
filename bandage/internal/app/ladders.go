package app

import (
	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/report"
)

// ruleStances resolves report stances by rule from the loaded chains -
// the report service's Downgrade seam (spec: Downgrade). A rung's
// stance is its first report action: under suppression that is the
// write that lands when the rung fires, so a later one never writes a
// stance. Reasons are namespaced at assembly, as at write time
// (ADR 0016).
type ruleStances map[string][]report.Stance

func newRuleStances(chains []domain.Chain, reasonPrefix string) ruleStances {
	out := make(ruleStances)
	for _, chain := range chains {
		for _, rule := range chain.Rules {
			var stances []report.Stance
			for _, rung := range rule.Escalations {
				for _, a := range rung.Actions {
					if a.Report == nil {
						continue
					}
					stances = append(stances, report.Stance{
						At:      rung.At,
						Reason:  domain.NamespacedReason(reasonPrefix, a.Report.Reason),
						Penalty: a.Report.Penalty,
						TTL:     a.Report.TTL,
						Margin:  a.Report.Margin,
					})
					break
				}
			}
			if len(stances) > 0 {
				out[rule.ID] = stances
			}
		}
	}
	return out
}

func (r ruleStances) Stances(rule string) []report.Stance { return r[rule] }
