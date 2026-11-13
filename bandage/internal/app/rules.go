package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/filter"
	"github.com/HT4w5/webmedkit/bandage/internal/domain/track"
)

// reasonRe is the machine-readable reason slug pattern.
var reasonRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// raw rules wire types (strict decode).

type rawRules struct {
	Version *int `yaml:"version"`
	// ReasonPrefix is the optional report-namespace prefix; its node is
	// validated in buildReasonPrefix.
	ReasonPrefix yaml.Node  `yaml:"reason_prefix"`
	Chains       []rawChain `yaml:"chains"`
}

type rawChain struct {
	Name  string    `yaml:"name"`
	Rules []rawRule `yaml:"rules"`
}

// rawRule carries the rule's keys; counter is a yaml.Node so presence
// is validated (default false).
type rawRule struct {
	ID      string      `yaml:"id"`
	Filter  string      `yaml:"filter"`
	Counter yaml.Node   `yaml:"counter"`
	Tracker *rawTracker `yaml:"tracker"`
	// Actions is the rule's action list (nil key absent, empty list a
	// declared empty list).
	Actions *[]yaml.Node `yaml:"actions"`
	// Escalations is the rule's ladder (nil key absent, empty list a declared empty ladder).
	Escalations *[]rawEscalation `yaml:"escalations"`
}

// rawEscalation is one ladder rung: a threshold in the tracker's quantity plus its action list.
type rawEscalation struct {
	At      yaml.Node   `yaml:"at"`
	Actions []yaml.Node `yaml:"actions"`
}

// rawTracker keeps leak and volume as yaml.Nodes so presence is
// validated and a removed volume-method block is detectable (spec:
// Volume-method removal).
type rawTracker struct {
	Track        string          `yaml:"track"`
	Leak         yaml.Node       `yaml:"leak"`
	Volume       yaml.Node       `yaml:"volume"`
	TargetPrefix rawTargetPrefix `yaml:"target_prefix"`
}

// rawTargetPrefix is the tracker's per-family target aggregation;
// missing families default to {32, 64}.
type rawTargetPrefix struct {
	IPv4 *int `yaml:"ipv4"`
	IPv6 *int `yaml:"ipv6"`
}

// rawReportAction is the report action's block; ttl and grace are
// yaml.Nodes so presence is validated - absent ttl means a dynamic
// report (spec: Two lifecycle modes), a written key must carry a
// duration.
type rawReportAction struct {
	Reason  string    `yaml:"reason"`
	TTL     yaml.Node `yaml:"ttl"`
	Grace   yaml.Node `yaml:"grace"`
	Penalty string    `yaml:"penalty"`
	Rate    yaml.Node `yaml:"rate"`
}

// Rules is a validated rules document: its chains and the optional
// reason prefix.
type Rules struct {
	Chains []domain.Chain
	// ReasonPrefix is the configured report-namespace prefix; empty
	// means the key is absent and the program-name fallback applies at
	// daemon assembly and in the plan printer (ADR 0026).
	ReasonPrefix string
}

// effectiveReasonPrefix is the prefix report reasons namespace under:
// the configured reason_prefix, else the program name (ADR 0026).
func (r Rules) effectiveReasonPrefix(progName string) string {
	if r.ReasonPrefix != "" {
		return r.ReasonPrefix
	}
	return progName
}

// LoadRules reads and validates the rules file at path.
func LoadRules(path string) (Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Rules{}, fmt.Errorf("rules: read %s: %w", path, err)
	}
	doc, err := ParseRules(data)
	if err != nil {
		return Rules{}, fmt.Errorf("rules %s: %w", path, err)
	}
	return doc, nil
}

// ParseRules strictly decodes and validates a format-version-0 rules
// document: its chains and their actions. Errors name the rule or key
// they belong to.
func ParseRules(data []byte) (Rules, error) {
	var raw rawRules
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return Rules{}, errors.New("rules: empty document")
		}
		return Rules{}, fmt.Errorf("rules: decode: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return Rules{}, errors.New("rules: multiple YAML documents")
	} else if !errors.Is(err, io.EOF) {
		return Rules{}, fmt.Errorf("rules: decode: %w", err)
	}

	if raw.Version == nil {
		return Rules{}, errors.New("rules: version: required (want 0)")
	}
	if *raw.Version != 0 {
		return Rules{}, fmt.Errorf("rules: version: unsupported format version %d (want 0)", *raw.Version)
	}
	prefix, err := buildReasonPrefix(&raw.ReasonPrefix)
	if err != nil {
		return Rules{}, err
	}
	if len(raw.Chains) == 0 {
		return Rules{}, errors.New("rules: chains: at least one chain required")
	}

	seen := make(map[string]bool)
	chains := make([]domain.Chain, 0, len(raw.Chains))
	for i := range raw.Chains {
		chain, err := buildChain(&raw.Chains[i], seen)
		if err != nil {
			return Rules{}, err
		}
		chains = append(chains, chain)
	}
	if err := validateChainGraph(chains); err != nil {
		return Rules{}, err
	}
	return Rules{Chains: chains, ReasonPrefix: prefix}, nil
}

// keyWritten reports whether the key was written at all. One shared
// rule for the optional keys (top-level reason_prefix, rule-level
// counter): an absent key means the default, while a written key - a
// written null included - must carry a valid value and fails load
// otherwise (a written null is a typo, not an absent key).
func keyWritten(node *yaml.Node) bool {
	return node != nil && node.Kind != 0
}

// buildReasonPrefix validates the optional top-level reason_prefix:
// a written key must carry a valid string slug (a written null or
// non-string scalar is a typo and fails load naming the key), an
// absent key means no configured prefix (the program-name fallback,
// ADR 0026).
func buildReasonPrefix(node *yaml.Node) (string, error) {
	if !keyWritten(node) {
		return "", nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", errors.New("rules: reason_prefix: must be a slug")
	}
	if !reasonRe.MatchString(node.Value) {
		return "", fmt.Errorf("rules: reason_prefix: %q is not a valid slug (want %s)", node.Value, reasonRe.String())
	}
	return node.Value, nil
}

func buildChain(raw *rawChain, seen map[string]bool) (domain.Chain, error) {
	if raw.Name == "" {
		return domain.Chain{}, errors.New("rules: chain: name: required")
	}
	if len(raw.Rules) == 0 {
		return domain.Chain{}, fmt.Errorf("rules: chain %q: at least one rule required", raw.Name)
	}
	chain := domain.Chain{Name: raw.Name, Rules: make([]domain.Rule, 0, len(raw.Rules))}
	for i := range raw.Rules {
		rule, err := buildRule(raw.Name, &raw.Rules[i], seen)
		if err != nil {
			return domain.Chain{}, err
		}
		chain.Rules = append(chain.Rules, rule)
	}
	return chain, nil
}

func buildRule(chainName string, raw *rawRule, seen map[string]bool) (domain.Rule, error) {
	if raw.ID == "" {
		return domain.Rule{}, fmt.Errorf("rules: chain %q: rule: id: required", chainName)
	}
	id := raw.ID
	if seen[id] {
		return domain.Rule{}, fmt.Errorf("rules: duplicate rule id %q", id)
	}
	seen[id] = true

	rule := domain.Rule{ID: id}

	counter, err := parseCounter(id, &raw.Counter)
	if err != nil {
		return domain.Rule{}, err
	}
	rule.Counter = counter

	if raw.Escalations != nil && raw.Actions != nil {
		return domain.Rule{}, fmt.Errorf("rules: rule %q: escalations: cannot be declared beside actions", id)
	}
	if raw.Escalations != nil && raw.Tracker == nil {
		return domain.Rule{}, fmt.Errorf("rules: rule %q: escalations: requires a tracker", id)
	}

	flt := raw.Filter
	if flt == "" {
		flt = "any"
	}
	parsed, err := filter.Parse(flt)
	if err != nil {
		return domain.Rule{}, fmt.Errorf("rules: rule %q: %w", id, err)
	}
	rule.Filter = parsed
	rule.FilterSource = flt

	if raw.Tracker != nil {
		var (
			cfg *domain.TrackerConfig
			err error
		)
		if raw.Escalations != nil {
			cfg, err = parseLadderTracker(id, raw.Tracker)
		} else {
			cfg, err = parseTracker(id, raw.Tracker)
		}
		if err != nil {
			return domain.Rule{}, err
		}
		rule.Tracker = cfg
	}

	if raw.Escalations != nil {
		rungs, err := parseEscalations(id, *raw.Escalations, rule.Tracker.Kind)
		if err != nil {
			return domain.Rule{}, err
		}
		rule.Escalations = rungs
		// The rungs stand in for tracker.volume: the cache-entry TTL derives
		// from the top rung so state outlives the whole climb; replay re-derives per event and keeps the build TTL zero.
		if top, ok := rule.TopRung(); ok && rule.Tracker.Kind != domain.TrackReplay {
			rule.Tracker.TTL = domain.TTL(rule.Tracker.Leak, top.At)
		}
	}

	var actions []yaml.Node
	if raw.Actions != nil {
		actions = *raw.Actions
	}
	for i := range actions {
		action, err := parseAction(id, &actions[i])
		if err != nil {
			return domain.Rule{}, err
		}
		// A report action needs no tracker; suppression bounds
		// tracker-less writes.
		rule.Actions = append(rule.Actions, action)
	}
	if err := validateReportLifecycles(rule); err != nil {
		return domain.Rule{}, err
	}
	compileGraceMargins(&rule)
	return rule, nil
}

// compileGraceMargins compiles every report action's grace to its
// margin: leak x grace in the tracker's quantity (spec: Report action
// grammar - at rule build, grace compiles to a margin). The death line
// is the recorded volume minus this margin.
func compileGraceMargins(rule *domain.Rule) {
	for _, actions := range rule.ActionLists() {
		for _, action := range actions {
			if a := action.Report; a != nil && a.Grace != 0 {
				a.Margin = a.Grace.Seconds() * rule.Tracker.Leak
			}
		}
	}
}

// validateReportLifecycles refuses a report action without ttl wherever
// no tracker value can ever kill it: a tracker-less rule or a zero-leak
// tracker can never underflow, so the report would be unbounded (spec:
// Load validation additions). Errors name the rule and the action's
// reason.
func validateReportLifecycles(rule domain.Rule) error {
	for _, actions := range rule.ActionLists() {
		for _, action := range actions {
			a := action.Report
			if a == nil || a.TTL != 0 {
				continue
			}
			switch {
			case rule.Tracker == nil:
				return fmt.Errorf("rules: rule %q: action report %q: report.ttl: required - a dynamic report lives and dies with its tracker value, and this rule has no tracker", rule.ID, a.Reason)
			case rule.Tracker.Leak <= 0:
				return fmt.Errorf("rules: rule %q: action report %q: report.ttl: required - a dynamic report lives and dies with its tracker value, and a zero-leak tracker can never underflow", rule.ID, a.Reason)
			}
		}
	}
	return nil
}

// parseCounter parses the rule's optional counter flag: true enables
// the rule's Rule counter (GLOSSARY: Rule counter). The shared
// written-key rule applies: an absent key means the default false, a
// written key must carry a boolean - a written null is a typo and
// fails load like any other invalid value, naming the key.
func parseCounter(id string, node *yaml.Node) (bool, error) {
	if !keyWritten(node) {
		return false, nil
	}
	var on bool
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || node.Decode(&on) != nil {
		return false, fmt.Errorf("rules: rule %q: counter: must be true or false", id)
	}
	return on, nil
}

// parseTracker builds the single-volume rule's tracker config: one plain
// scalar volume in the tracker kind's quantity. Rate and bandwidth fix
// the TTL at build, replay derives it per event so TTL stays zero.
func parseTracker(id string, raw *rawTracker) (*domain.TrackerConfig, error) {
	cfg, err := parseTrackerBase(id, raw)
	if err != nil {
		return nil, err
	}
	volume, err := parseTrackerQuantity(id, "tracker.volume", &raw.Volume, cfg.Kind)
	if err != nil {
		return nil, err
	}
	if volume <= 0 {
		return nil, fmt.Errorf("rules: rule %q: tracker.volume: must be positive", id)
	}
	cfg.Volume = volume
	if cfg.Kind != domain.TrackReplay {
		cfg.TTL = domain.TTL(cfg.Leak, volume)
	}
	return cfg, nil
}

// parseLadderTracker builds the ladder rule's tracker config: a ladder carries
// no tracker.volume (its rungs stand in for it), so any volume beside it is refused.
func parseLadderTracker(id string, raw *rawTracker) (*domain.TrackerConfig, error) {
	cfg, err := parseTrackerBase(id, raw)
	if err != nil {
		return nil, err
	}
	if keyWritten(&raw.Volume) {
		return nil, fmt.Errorf("rules: rule %q: escalations: cannot be declared beside tracker.volume", id)
	}
	return cfg, nil
}

func parseTrackerBase(id string, raw *rawTracker) (*domain.TrackerConfig, error) {
	var kind domain.TrackKind
	switch raw.Track {
	case "":
		return nil, fmt.Errorf("rules: rule %q: tracker.track: required (want %q, %q or %q)",
			id, domain.TrackRate, domain.TrackBandwidth, domain.TrackReplay)
	case string(domain.TrackRate), string(domain.TrackBandwidth), string(domain.TrackReplay):
		kind = domain.TrackKind(raw.Track)
	default:
		return nil, fmt.Errorf("rules: rule %q: tracker.track: unknown tracker kind %q (want %q, %q or %q)",
			id, raw.Track, domain.TrackRate, domain.TrackBandwidth, domain.TrackReplay)
	}

	leak, err := parseTrackerQuantity(id, "tracker.leak", &raw.Leak, kind)
	if err != nil {
		return nil, err
	}
	if leak < 0 {
		return nil, fmt.Errorf("rules: rule %q: tracker.leak: must be non-negative", id)
	}
	prefix, err := parseTargetPrefix(id, &raw.TargetPrefix)
	if err != nil {
		return nil, err
	}
	return &domain.TrackerConfig{Kind: kind, Leak: leak, TargetPrefix: prefix}, nil
}

// parseTargetPrefix builds the per-family target aggregation, default
// {32, 64}; values must fit the family (IPv4 0-32, IPv6 0-64).
func parseTargetPrefix(id string, raw *rawTargetPrefix) (domain.TargetPrefix, error) {
	prefix := domain.DefaultTargetPrefix
	if raw.IPv4 != nil {
		if *raw.IPv4 < 0 || *raw.IPv4 > 32 {
			return domain.TargetPrefix{}, fmt.Errorf("rules: rule %q: tracker.target_prefix.ipv4: must be between 0 and 32, got %d", id, *raw.IPv4)
		}
		prefix.IPv4 = *raw.IPv4
	}
	if raw.IPv6 != nil {
		if *raw.IPv6 < 0 || *raw.IPv6 > 64 {
			return domain.TargetPrefix{}, fmt.Errorf("rules: rule %q: tracker.target_prefix.ipv6: must be between 0 and 64, got %d", id, *raw.IPv6)
		}
		prefix.IPv6 = *raw.IPv6
	}
	return prefix, nil
}

// parseTrackerQuantity parses one scalar tracker quantity: plain
// numbers for rate and replay, byte quantities for bandwidth.
func parseTrackerQuantity(id, field string, node *yaml.Node, kind domain.TrackKind) (float64, error) {
	if node == nil || node.Kind == 0 {
		return 0, fmt.Errorf("rules: rule %q: %s: required", id, field)
	}
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("rules: rule %q: %s: must be a quantity", id, field)
	}
	var (
		v   float64
		err error
	)
	if kind == domain.TrackBandwidth {
		v, err = track.ParseBytes(node.Value)
	} else {
		v, err = track.ParseNumber(node.Value)
	}
	if err != nil {
		return 0, fmt.Errorf("rules: rule %q: %s: %w", id, field, err)
	}
	return v, nil
}

// parseEscalations builds the rule's ladder: at least two rungs with strictly
// ascending thresholds, so the rungs a value exceeds stay a ladder prefix.
func parseEscalations(id string, raw []rawEscalation, kind domain.TrackKind) ([]domain.Escalation, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("rules: rule %q: escalations: at least two rungs required", id)
	}
	rungs := make([]domain.Escalation, len(raw))
	for i := range raw {
		at, err := parseTrackerQuantity(id, fmt.Sprintf("escalations[%d].at", i), &raw[i].At, kind)
		if err != nil {
			return nil, err
		}
		if i > 0 && at <= rungs[i-1].At {
			return nil, fmt.Errorf("rules: rule %q: escalations[%d].at: must be strictly ascending", id, i)
		}
		rung := domain.Escalation{At: at}
		for j := range raw[i].Actions {
			action, err := parseAction(id, &raw[i].Actions[j])
			if err != nil {
				return nil, err
			}
			rung.Actions = append(rung.Actions, action)
		}
		rungs[i] = rung
	}
	return rungs, nil
}

// actionForms is the format-version-0 action vocabulary; a nil node is
// the bare form (`- stop`).
var actionForms = map[string]func(id, name string, val *yaml.Node) (domain.Action, error){
	"report": parseReportAction,
	"log":    parseLogAction,
	"enrich": parseEnrichAction,
	"goto":   parseChainAction,
	"jump":   parseChainAction,
	"stop":   parseStopAction,
}

// parseAction parses one action item: a bare name (`- stop`) or a name
// with options.
func parseAction(id string, node *yaml.Node) (domain.Action, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		parse := actionForms[node.Value]
		if parse == nil {
			return domain.Action{}, fmt.Errorf("rules: rule %q: unknown action %q", id, node.Value)
		}
		return parse(id, node.Value, nil)
	case yaml.MappingNode:
		if len(node.Content) != 2 {
			return domain.Action{}, fmt.Errorf("rules: rule %q: action must be a single name with options", id)
		}
		name, val := node.Content[0].Value, node.Content[1]
		parse := actionForms[name]
		if parse == nil {
			return domain.Action{}, fmt.Errorf("rules: rule %q: unknown action %q", id, name)
		}
		return parse(id, name, val)
	default:
		return domain.Action{}, fmt.Errorf("rules: rule %q: action must be a name or a name with options", id)
	}
}

// wantsOptions is the bare-form error for actions that take options.
func wantsOptions(id, name string) error {
	return fmt.Errorf("rules: rule %q: action %q: wants options", id, name)
}

// parseStopAction parses the bare `- stop` action; it takes no options.
func parseStopAction(id, name string, val *yaml.Node) (domain.Action, error) {
	if val != nil {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action stop takes no options", id)
	}
	return domain.Action{Stop: true}, nil
}

// rawEnrichAction is the enrich action's block; type is the only
// required key (no per-action enrichment configuration, ADR 0028),
// counter the optional Enrichment counter opt-in.
type rawEnrichAction struct {
	Type    string    `yaml:"type"`
	Counter yaml.Node `yaml:"counter"`
}

// parseEnrichAction parses `- enrich: {type, counter}`: one enrichment
// kind by name, run where the rule matches (ADR 0028), and the pair's
// Enrichment counter opt-in (GLOSSARY: Enrichment counter). The strict
// key set is deliberately tiny: unknown keys fail load naming them.
// The block's shape is validated before its vocabulary, so a malformed
// block still names what it rejects; with the kind vocabulary empty
// every enrich action then fails load, naming its type and stating
// that no enrichment types are available.
func parseEnrichAction(id, name string, val *yaml.Node) (domain.Action, error) {
	if val == nil {
		return domain.Action{}, wantsOptions(id, name)
	}
	for i := 0; i < len(val.Content); i += 2 {
		switch key := val.Content[i].Value; key {
		case "type", "counter":
		default:
			return domain.Action{}, fmt.Errorf("rules: rule %q: action enrich: unknown key %q", id, key)
		}
	}
	var raw rawEnrichAction
	if err := val.Decode(&raw); err != nil {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action enrich: %w", id, err)
	}
	counter, err := parseEnrichCounter(id, &raw.Counter)
	if err != nil {
		return domain.Action{}, err
	}
	if raw.Type == "" {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action enrich: type: required %s", id, enrichmentTypeWant)
	}
	if !knownEnrichmentType(raw.Type) {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action enrich: type: unknown enrichment type %q %s", id, raw.Type, enrichmentTypeWant)
	}
	return domain.Action{Enrich: &domain.EnrichAction{Type: raw.Type, Counter: counter}}, nil
}

// parseEnrichCounter parses the enrich action's optional counter flag:
// true enables the (rule, enrichment) pair's Enrichment counter
// (GLOSSARY: Enrichment counter). The shared written-key rule applies:
// an absent key means the default false, a written key must carry a
// boolean - a written null is a typo and fails load like any other
// invalid value, naming the key.
func parseEnrichCounter(id string, node *yaml.Node) (bool, error) {
	if !keyWritten(node) {
		return false, nil
	}
	var on bool
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || node.Decode(&on) != nil {
		return false, fmt.Errorf("rules: rule %q: action enrich: counter: must be true or false", id)
	}
	return on, nil
}

// parseReportAction parses `- report: {reason, ttl, grace, penalty,
// rate}`: ttl and grace are optional and mutually exclusive - an
// absent ttl declares a dynamic report (spec: Two lifecycle modes),
// whose liveness validateReportLifecycles checks against the rule's
// tracker, and whose grace widens its death line (spec: Report action
// grammar).
func parseReportAction(id, name string, val *yaml.Node) (domain.Action, error) {
	if val == nil {
		return domain.Action{}, wantsOptions(id, name)
	}
	var raw rawReportAction
	hasRate := false
	for i := 0; i < len(val.Content); i += 2 {
		switch key := val.Content[i].Value; key {
		case "reason", "ttl", "grace", "penalty":
		case "rate":
			hasRate = true
		default:
			return domain.Action{}, fmt.Errorf("rules: rule %q: action report: unknown key %q", id, key)
		}
	}
	if err := val.Decode(&raw); err != nil {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action report: %w", id, err)
	}
	if !reasonRe.MatchString(raw.Reason) {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action report: reason: %q is not a valid slug (want %s)",
			id, raw.Reason, reasonRe.String())
	}
	var ttl time.Duration
	if keyWritten(&raw.TTL) {
		var err error
		ttl, err = parseDuration(raw.TTL.Value)
		if err != nil {
			return domain.Action{}, fmt.Errorf("rules: rule %q: action report: report.ttl: %w", id, err)
		}
	}
	var grace time.Duration
	if keyWritten(&raw.Grace) {
		var err error
		grace, err = parseDuration(raw.Grace.Value)
		if err != nil {
			return domain.Action{}, fmt.Errorf("rules: rule %q: action report: report.grace: %w", id, err)
		}
	}
	if ttl != 0 && grace != 0 {
		// One lifecycle policy per report action: ttl and grace are
		// mutually exclusive (spec: Report action grammar).
		return domain.Action{}, fmt.Errorf("rules: rule %q: action report: report.grace: not allowed with report.ttl", id)
	}
	penalty, err := parsePenalty(id, &raw, hasRate)
	if err != nil {
		return domain.Action{}, err
	}
	return domain.Action{Report: &domain.ReportAction{Reason: raw.Reason, TTL: ttl, Grace: grace, Penalty: penalty}}, nil
}

// penaltyWant names the closed penalty vocabulary for error messages,
// built from the domain's single source (domain.PenaltyActions).
var penaltyWant = func() string {
	names := make([]string, len(domain.PenaltyActions))
	for i, action := range domain.PenaltyActions {
		names[i] = fmt.Sprintf("%q", action)
	}
	last := len(names) - 1
	return "(want " + strings.Join(names[:last], ", ") + " or " + names[last] + ")"
}()

// parsePenalty builds the report action's penalty: one of the closed
// values of ADR 0010 (domain.PenaltyActions), with a rate exactly on
// the limit penalties. Well-formedness only; never checked against the
// rule's tracker or reason.
func parsePenalty(id string, raw *rawReportAction, hasRate bool) (domain.Penalty, error) {
	action := domain.PenaltyAction(raw.Penalty)
	penalty := domain.Penalty{Action: action}
	switch {
	case action == "":
		return domain.Penalty{}, fmt.Errorf("rules: rule %q: action report: penalty: required %s", id, penaltyWant)
	case !action.IsValid():
		return domain.Penalty{}, fmt.Errorf("rules: rule %q: action report: penalty: unknown penalty %q %s", id, raw.Penalty, penaltyWant)
	case !action.IsLimit():
		if hasRate {
			return domain.Penalty{}, fmt.Errorf("rules: rule %q: action report: rate: not allowed with penalty %q", id, action)
		}
		return penalty, nil
	}
	if !hasRate {
		return domain.Penalty{}, fmt.Errorf("rules: rule %q: action report: rate: required with penalty %q", id, action)
	}
	rate, err := parseRate(id, raw.Rate)
	if err != nil {
		return domain.Penalty{}, err
	}
	penalty.Rate = &rate
	return penalty, nil
}

// parseRate parses a limit penalty's per-second threshold: a plain
// count or a decimal human size (1MB = 1e6) in the unit the penalty
// name states, integral and at least 1.
func parseRate(id string, node yaml.Node) (int64, error) {
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("rules: rule %q: action report: rate: must be a quantity", id)
	}
	v, err := track.ParseBytes(node.Value)
	if err != nil {
		return 0, fmt.Errorf("rules: rule %q: action report: rate: %w", id, err)
	}
	if v != math.Trunc(v) {
		return 0, fmt.Errorf("rules: rule %q: action report: rate: must be a whole number, got %q", id, node.Value)
	}
	if v < 1 {
		return 0, fmt.Errorf("rules: rule %q: action report: rate: must be positive, got %q", id, node.Value)
	}
	if v >= float64(math.MaxInt64) {
		return 0, fmt.Errorf("rules: rule %q: action report: rate: out of range, got %q", id, node.Value)
	}
	return int64(v), nil
}

// parseChainAction parses `goto: <chain>` and `jump: <chain>`; targets
// are validated by validateChainGraph.
func parseChainAction(id, name string, val *yaml.Node) (domain.Action, error) {
	if val == nil {
		return domain.Action{}, wantsOptions(id, name)
	}
	var target string
	if err := val.Decode(&target); err != nil || target == "" {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action %s: wants a chain name", id, name)
	}
	if name == "goto" {
		return domain.Action{Goto: target}, nil
	}
	return domain.Action{Jump: target}, nil
}

// parseLogAction parses `log: "level,message"`: a level and a message.
func parseLogAction(id, name string, val *yaml.Node) (domain.Action, error) {
	if val == nil {
		return domain.Action{}, wantsOptions(id, name)
	}
	if val.Kind != yaml.ScalarNode {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action log: wants a \"level,message\" string", id)
	}
	args := strings.Split(val.Value, ",")
	if len(args) != 2 {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action log: wants \"level,message\", got %q", id, val.Value)
	}
	level, err := domain.ParseLevel(args[0])
	if err != nil {
		return domain.Action{}, fmt.Errorf("rules: rule %q: action log: %w", id, err)
	}
	return domain.Action{Log: &domain.LogAction{Level: level, Message: args[1]}}, nil
}

// parseDuration parses a report lifecycle duration (ttl, grace): Go
// duration strings plus "d" for days (7d, 24h, 30m). It must be
// positive.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, errors.New("required")
	}
	d, err := time.ParseDuration(s)
	if err != nil && strings.HasSuffix(s, "d") {
		// Day counts (7d) are not Go durations.
		var days int64
		if days, err = strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 64); err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		d = time.Duration(days) * 24 * time.Hour
	}
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %q", s)
	}
	return d, nil
}
