package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
	"github.com/HT4w5/webmedkit/internal/identity"
)

// Rules file (format version 0) and its load-time validation: every rule
// gets a failing and a passing case.
const validRules = `
version: 0

chains:
  - name: main
    rules:
      - id: sustained-iso-egress
        filter: any
        tracker:
          track: bandwidth
          leak: 1MB
          volume: 100GB
          target_prefix: {ipv4: 24, ipv6: 64}
        actions:
          - report:
              reason: iso-egress-overflow
              ttl: 7d
              penalty: limit-bandwidth
              rate: 1MB

  - name: api
    rules:
      - id: bursty-api
        tracker:
          track: rate
          leak: 10
          volume: 100
        actions:
          - report:
              reason: api-rate-overflow
              ttl: 24h
              penalty: limit-req-rate
              rate: 200
`

func TestParseRulesValid(t *testing.T) {
	doc, err := ParseRules([]byte(validRules))
	if err != nil {
		t.Fatalf("ParseRules error: %v", err)
	}
	chains := doc.Chains
	if len(chains) != 2 {
		t.Fatalf("chains = %d, want 2", len(chains))
	}

	iso := chains[0].Rules[0]
	if iso.ID != "sustained-iso-egress" {
		t.Errorf("rule id = %q", iso.ID)
	}
	if !iso.Filter.Match(&domain.Event{}) {
		t.Error("filter does not match every event")
	}
	if iso.Tracker == nil {
		t.Fatal("tracker = nil")
	}
	if iso.Tracker.Kind != domain.TrackBandwidth {
		t.Errorf("tracker kind = %q, want bandwidth", iso.Tracker.Kind)
	}
	if iso.Tracker.Leak != 1e6 || iso.Tracker.Volume != 1e11 {
		t.Errorf("tracker leak/volume = %v/%v, want 1e6/1e11", iso.Tracker.Leak, iso.Tracker.Volume)
	}
	if iso.Tracker.TTL != 1e5*time.Second {
		t.Errorf("tracker TTL = %v, want %v", iso.Tracker.TTL, 1e5*time.Second)
	}
	if iso.Tracker.TargetPrefix != (domain.TargetPrefix{IPv4: 24, IPv6: 64}) {
		t.Errorf("tracker target_prefix = %+v, want {24 64}", iso.Tracker.TargetPrefix)
	}
	if len(iso.Actions) != 1 || iso.Actions[0].Report == nil {
		t.Fatalf("actions = %+v, want one report action", iso.Actions)
	}
	if iso.Actions[0].Report.Reason != "iso-egress-overflow" || iso.Actions[0].Report.TTL != 168*time.Hour {
		t.Errorf("report action = %+v", iso.Actions[0].Report)
	}
	if p := iso.Actions[0].Report.Penalty; p.Action != domain.PenaltyLimitBandwidth || p.Rate == nil || *p.Rate != 1000000 {
		t.Errorf("report penalty = %+v, want limit-bandwidth rate 1000000", p)
	}

	api := chains[1].Rules[0]
	if api.Tracker == nil || api.Tracker.Kind != domain.TrackRate {
		t.Fatalf("api tracker = %+v, want rate", api.Tracker)
	}
	if api.Tracker.Leak != 10 || api.Tracker.Volume != 100 {
		t.Errorf("api tracker leak/volume = %v/%v, want 10/100", api.Tracker.Leak, api.Tracker.Volume)
	}
	if api.Tracker.TTL != 10*time.Second {
		t.Errorf("api tracker TTL = %v, want 10s", api.Tracker.TTL)
	}
	if api.Tracker.TargetPrefix != domain.DefaultTargetPrefix {
		t.Errorf("tracker target_prefix = %+v, want default %+v", api.Tracker.TargetPrefix, domain.DefaultTargetPrefix)
	}
}

// TestParseRulesTargetPrefixDefaults: target_prefix defaults to {32, 64} and
// each family defaults independently when only the other is declared.
func TestParseRulesTargetPrefixDefaults(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  domain.TargetPrefix
	}{
		{"omitted defaults to {32, 64}", "", domain.DefaultTargetPrefix},
		{"ipv4 only defaults ipv6", "target_prefix: {ipv4: 24}\n", domain.TargetPrefix{IPv4: 24, IPv6: 64}},
		{"ipv6 only defaults ipv4", "target_prefix: {ipv6: 48}\n", domain.TargetPrefix{IPv4: 32, IPv6: 48}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker:\n          track: rate\n          leak: 1\n          volume: 2\n          " + tt.extra
			doc, err := ParseRules([]byte(yaml))
			if err != nil {
				t.Fatalf("ParseRules error: %v", err)
			}
			if got := doc.Chains[0].Rules[0].Tracker.TargetPrefix; got != tt.want {
				t.Errorf("target_prefix = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseRulesReasonPrefix: the optional top-level reason_prefix is
// carried through slug-validated; an absent key carries no configured
// prefix (the program-name fallback is chosen where the program name
// is known - daemon assembly and the plan printer, ADR 0026). A
// written key must carry a valid string slug: a written null or
// non-string scalar is a typo and fails load naming the key, and
// string errors quote the authored value.
func TestParseRulesReasonPrefix(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string // the authored reason_prefix line ("" = the key is absent)
		want    string
		wantErr string
	}{
		{name: "absent key carries no configured prefix"},
		{name: "slug prefix carried through", prefix: "reason_prefix: acme-prod", want: "acme-prod"},
		{name: "digits and underscores carried through", prefix: "reason_prefix: acme_2_prod", want: "acme_2_prod"},
		{name: "dotted value refused quoting the authored value", prefix: "reason_prefix: Acme.Prod", wantErr: `rules: reason_prefix: "Acme.Prod" is not a valid slug (want `},
		{name: "uppercase value refused", prefix: "reason_prefix: Acme", wantErr: `rules: reason_prefix: "Acme" is not a valid slug (want `},
		{name: "spaced value refused", prefix: `reason_prefix: "has space"`, wantErr: `rules: reason_prefix: "has space" is not a valid slug (want `},
		{name: "empty value refused", prefix: `reason_prefix: ""`, wantErr: `rules: reason_prefix: "" is not a valid slug (want `},
		// A written key must carry a valid string slug: a written null or
		// non-string scalar is a typo and fails load with the key-naming
		// error - only an absent key means the default.
		{name: "null value refused", prefix: "reason_prefix:", wantErr: `rules: reason_prefix: must be a slug`},
		{name: "explicit null value refused", prefix: "reason_prefix: null", wantErr: `rules: reason_prefix: must be a slug`},
		{name: "tilde null value refused", prefix: "reason_prefix: ~", wantErr: `rules: reason_prefix: must be a slug`},
		{name: "numeric value refused as a non-string", prefix: "reason_prefix: 123", wantErr: `rules: reason_prefix: must be a slug`},
		{name: "boolean value refused as a non-string", prefix: "reason_prefix: true", wantErr: `rules: reason_prefix: must be a slug`},
		{name: "sequence value refused", prefix: "reason_prefix: [a]", wantErr: `rules: reason_prefix: must be a slug`},
		{name: "quoted digits are a string slug", prefix: `reason_prefix: "123"`, want: "123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "version: 0\n"
			if tt.prefix != "" {
				src += tt.prefix + "\n"
			}
			src += "chains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: drop}}]\n"
			doc, err := ParseRules([]byte(src))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseRules succeeded, want error naming the key %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRules error: %v", err)
			}
			if doc.ReasonPrefix != tt.want {
				t.Errorf("ReasonPrefix = %q, want %q", doc.ReasonPrefix, tt.want)
			}
		})
	}
}

// TestParseRulesCounterFlag: the rule-level counter key enables the
// rule's Rule counter (GLOSSARY: Rule counter); it defaults to false,
// so upgrading adds no noise until asked for.
func TestParseRulesCounterFlag(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  bool
	}{
		{"omitted defaults to false", "", false},
		{"true enables the rule counter", "counter: true\n", true},
		{"false keeps the rule counter off", "counter: false\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        " + tt.extra
			doc, err := ParseRules([]byte(yaml))
			if err != nil {
				t.Fatalf("ParseRules error: %v", err)
			}
			if got := doc.Chains[0].Rules[0].Counter; got != tt.want {
				t.Errorf("counter = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseRulesEnrichActionCounterKeyShape: with the kind vocabulary
// empty the enrich action's counter key keeps its documented shape
// (GLOSSARY: Enrichment counter) - a well-formed block rides the
// vocabulary refusal naming its type, a written non-boolean fails load
// naming the key.
func TestParseRulesEnrichActionCounterKeyShape(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{"omitted rides the vocabulary refusal", "", `unknown enrichment type "filesize" (no enrichment types are available)`},
		{"true rides the vocabulary refusal", ", counter: true", `unknown enrichment type "filesize" (no enrichment types are available)`},
		{"false rides the vocabulary refusal", ", counter: false", `unknown enrichment type "filesize" (no enrichment types are available)`},
		{"a written non-boolean names the key", `, counter: "true"`, "action enrich: counter: must be true or false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize" + tt.extra + "}}]\n"
			_, err := ParseRules([]byte(yaml))
			if err == nil {
				t.Fatalf("ParseRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRulesValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		// The four checklist validations: failing and passing cases.
		{
			name: "duplicate rule ids rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: dupe
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
  - name: b
    rules:
      - id: dupe
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: "duplicate rule id",
		},
		{
			name: "unique rule ids accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: one
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
      - id: two
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
		},
		{
			name: "report without tracker accepted (tracker-less tripwire)",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{report: {reason: x, ttl: 1h, penalty: drop}}]
`,
		},
		{
			name: "report with tracker accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
		},
		{
			name: "bad reason slug rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: Not A Slug, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: "reason",
		},
		{
			name: "reason leading dash rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: -x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: "reason",
		},
		{
			name: "reason with a dot rejected quoting the authored value",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: bandage.x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: `reason: "bandage.x"`,
		},
		{
			name: "good reason slug accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: iso-egress_overflow9, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
		},
		{
			name: "zero ttl rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 0s, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: "ttl",
		},
		{
			name: "negative ttl rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: -1h, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: "ttl",
		},
		{
			name: "missing ttl on a tracked rule accepted as a dynamic report",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, penalty: limit-req-rate, rate: 200}}]
`,
		},
		{
			name: "malformed ttl rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: banana, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: "ttl",
		},
		{
			name: "positive ttl accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 30m, penalty: limit-req-rate, rate: 200}}]
`,
		},

		// Document shape.
		{name: "missing version", yaml: "chains: []\n", wantErr: "version"},
		{name: "unsupported version", yaml: "version: 1\nchains: []\n", wantErr: "version"},
		{name: "missing chains", yaml: "version: 0\n", wantErr: "chain"},
		{name: "empty chains", yaml: "version: 0\nchains: []\n", wantErr: "chain"},
		{name: "chain without name", yaml: "version: 0\nchains:\n  - rules: [{id: r}]\n", wantErr: "name"},
		{name: "chain without rules", yaml: "version: 0\nchains:\n  - name: main\n", wantErr: "rule"},
		{name: "rule without id", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - filter: any\n", wantErr: "id"},
		{name: "unknown key at top level", yaml: "version: 0\nchainz: []\n", wantErr: "chainz"},
		{name: "unknown key in rule", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        buckets: []\n", wantErr: "buckets"},
		{name: "unknown key in tracker", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, ttl: 5}\n", wantErr: "ttl"},

		// Rule counter (rule-level counter key).
		{name: "string counter value rejected naming the key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        counter: sometimes\n", wantErr: `counter: must be true or false`},
		{name: "numeric counter value rejected naming the key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        counter: 1\n", wantErr: `counter: must be true or false`},
		{name: "sequence counter value rejected naming the key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        counter: [true]\n", wantErr: `counter: must be true or false`},
		{name: "quoted true counter value rejected naming the key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        counter: \"true\"\n", wantErr: `counter: must be true or false`},
		// A written null is a typo, not an absent key: the shared rule
		// with reason_prefix rejects it naming the key.
		{name: "null counter value rejected naming the key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        counter: null\n", wantErr: `counter: must be true or false`},
		{name: "empty counter value rejected naming the key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        counter:\n", wantErr: `counter: must be true or false`},

		// Filter.
		{name: "omitted filter defaults to any", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2}\n"},
		{name: "explicit any filter", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        filter: any\n        tracker: {track: rate, leak: 1, volume: 2}\n"},
		{
			name: "filter expression accepted",
			yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        filter: 'src 10.0.0.0/8'\n        tracker: {track: rate, leak: 1, volume: 2}\n",
		},

		// Tracker parsing and validation.
		{name: "missing track", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {leak: 1, volume: 2}\n", wantErr: "track"},
		{name: "unknown tracker kind", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: freq, leak: 1, volume: 2}\n", wantErr: "unknown tracker kind"},
		{name: "missing leak", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, volume: 2}\n", wantErr: "tracker.leak"},
		{name: "missing volume", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1}\n", wantErr: "tracker.volume"},
		{name: "zero volume", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 0}\n", wantErr: "tracker.volume"},
		{name: "negative leak", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: -1, volume: 2}\n", wantErr: "tracker.leak"},
		{name: "rate leak with byte unit", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1MB, volume: 2}\n", wantErr: "tracker.leak"},
		{name: "rate volume with byte unit", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 1MB}\n", wantErr: "tracker.volume"},
		{name: "bandwidth quantities accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: bandwidth, leak: 1MB/s, volume: 100GB}\n        actions: [{report: {reason: x, ttl: 7d, penalty: limit-bandwidth, rate: 1MB}}]\n"},
		{name: "bandwidth zero leak accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: bandwidth, leak: 0, volume: 1KB}\n"},
		{name: "bad quantity names rule and field", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: my-rule\n        tracker: {track: bandwidth, leak: 1, volume: 1q}\n", wantErr: `rule "my-rule": tracker.volume`},

		// Target aggregation (target_prefix).
		{name: "target prefix boundary values accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv4: 32, ipv6: 64}}\n"},
		{name: "zero target prefix accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv4: 0, ipv6: 0}}\n"},
		{name: "target prefix ipv4 above 32 rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv4: 33, ipv6: 64}}\n", wantErr: "target_prefix.ipv4"},
		{name: "target prefix ipv4 negative rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv4: -1}}\n", wantErr: "target_prefix.ipv4"},
		{name: "target prefix ipv6 finer than 64 rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv6: 65}}\n", wantErr: "target_prefix.ipv6"},
		{name: "target prefix ipv6 negative rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv6: -2}}\n", wantErr: "target_prefix.ipv6"},
		{name: "bad target prefix names rule and field", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: my-rule\n        tracker: {track: rate, leak: 1, volume: 2, target_prefix: {ipv6: 65}}\n", wantErr: `rule "my-rule": tracker.target_prefix.ipv6`},

		// Actions (report, log, goto, jump, stop) and their YAML shapes.
		{name: "unknown action rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2}\n        actions: [{ban: 15m}]\n", wantErr: "unknown action"},
		{name: "stop accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [stop]\n"},
		{
			name: "goto accepted and resolves target",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{goto: b}]
  - name: b
    rules:
      - id: s
        actions: [stop]
`,
		},
		{
			name: "jump accepted and resolves forward target",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{jump: later}]
  - name: later
    rules:
      - id: s
        actions: [stop]
`,
		},
		{name: "log accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{log: \"debug,ROUTE:\"}]\n"},
		{name: "routing and logging actions without tracker accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{log: \"info,seen\"}, {goto: b}]\n  - name: b\n    rules:\n      - id: s\n        actions: [stop]\n"},
		{name: "stop with options rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{stop: true}]\n", wantErr: "stop"},
		{name: "goto as scalar without options rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [goto]\n", wantErr: "goto"},
		{name: "goto without target rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{goto: \"\"}]\n", wantErr: "goto"},
		{name: "goto mapping target rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{goto: {chain: b}}]\n", wantErr: "goto"},
		{name: "jump without target rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{jump: null}]\n", wantErr: "jump"},
		{name: "log non-string rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{log: [debug, x]}]\n", wantErr: "level,message"},
		{name: "log wrong arg count rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{log: debug}]\n", wantErr: "level,message"},
		{name: "log bad level rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{log: \"verbose,x\"}]\n", wantErr: "level"},

		// The enrich action: reserved plumbing with an empty kind
		// vocabulary (ADR 0028) - the shape stays valid rules syntax, but
		// every enrich action fails load naming its type and stating that
		// no enrichment types are available.
		{name: "enrich fails load with no enrichment types", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize}}]\n", wantErr: `unknown enrichment type "filesize" (no enrichment types are available)`},
		{name: "enrich in rung actions fails load too", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1}\n        escalations:\n          - at: 5\n            actions: [{enrich: {type: filesize}}]\n          - at: 10\n            actions: [{enrich: {type: filesize}}, stop]\n", wantErr: `unknown enrichment type "filesize" (no enrichment types are available)`},
		{name: "enrich without options rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [enrich]\n", wantErr: "enrich"},
		{name: "enrich without type rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {}}]\n", wantErr: `action enrich: type: required`},
		{name: "enrich null type rejected", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: null}}]\n", wantErr: `action enrich: type: required`},
		// The Enrichment counter's counter key: optional (default false),
		// strictly boolean when written (GLOSSARY: Enrichment counter) -
		// a well-formed key rides the vocabulary refusal.
		{name: "enrich counter rides the vocabulary refusal", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: true}}]\n", wantErr: `no enrichment types are available`},
		{name: "enrich false counter rides the vocabulary refusal", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{enrich: {type: filesize, counter: false}}]\n", wantErr: `no enrichment types are available`},

		// The goto/jump chain graph: a chain named main (the entry
		// chain), no dispatch targeting main, unique chain names,
		// resolvable targets, acyclic (a DAG; shared target chains are
		// allowed).
		{
			name: "duplicate chain names rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: main
    rules:
      - id: s
        actions: [stop]
`,
			wantErr: "duplicate chain name",
		},
		{
			name: "goto dangling target rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{goto: nowhere}]
`,
			wantErr: "nonexistent chain",
		},
		{
			name: "jump dangling target rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{jump: nowhere}]
`,
			wantErr: "nonexistent chain",
		},
		{
			name: "goto self cycle rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: a
    rules:
      - id: s
        actions: [{goto: a}]
`,
			wantErr: "cycle",
		},
		{
			name: "goto jump cycle rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{jump: b}]
  - name: b
    rules:
      - id: s
        actions: [{goto: c}]
  - name: c
    rules:
      - id: t
        actions: [{goto: b}]
`,
			wantErr: "cycle",
		},
		{
			name: "jump self cycle rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: a
    rules:
      - id: s
        actions: [{jump: a}]
`,
			wantErr: "cycle",
		},
		{
			name: "document without a main chain rejected",
			yaml: `
version: 0
chains:
  - name: a
    rules:
      - id: r
        actions: [stop]
`,
			wantErr: `no chain named "main"`,
		},
		{
			name: "goto targeting the entry chain rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: b
    rules:
      - id: s
        actions: [{goto: main}]
`,
			wantErr: "targets the entry chain",
		},
		{
			name: "jump targeting the entry chain rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: b
    rules:
      - id: s
        actions: [{jump: main}]
`,
			wantErr: "targets the entry chain",
		},
		{
			name: "unreachable chain accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: draft
    rules:
      - id: s
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: draft, ttl: 1h, penalty: drop}}]
`,
		},
		{
			name: "shared target chain accepted as a DAG",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{goto: shared}]
  - name: two
    rules:
      - id: s
        actions: [{jump: shared}]
  - name: shared
    rules:
      - id: t
        actions: [{goto: sink}]
  - name: sink
    rules:
      - id: u
        actions: [stop]
`,
		},

		{name: "report without reason", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2}\n        actions: [{report: {ttl: 1h, penalty: limit-req-rate, rate: 200}}]\n", wantErr: "reason"},
		{name: "report with unknown key", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2}\n        actions: [{report: {reason: x, ttl: 1h, ban: 15m, penalty: limit-req-rate, rate: 200}}]\n", wantErr: "ban"},

		// Penalty (required, closed vocabulary): failing and passing
		// cases; the per-value passing cases are generated below from
		// the domain's single vocabulary source.
		{name: "report without penalty", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h}}]\n", wantErr: "penalty"},
		{name: "unknown penalty", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: ban}}]\n", wantErr: "unknown penalty"},

		// Rate on a penalty: limits only (failing and passing cases).
		{name: "rate with drop", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: drop, rate: 200}}]\n", wantErr: "not allowed"},
		{name: "rate with reject", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: reject, rate: 200}}]\n", wantErr: "not allowed"},
		{name: "limit-pkt-rate without rate", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-pkt-rate}}]\n", wantErr: "rate: required"},
		{name: "limit-bandwidth without rate", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth}}]\n", wantErr: "rate: required"},
		{name: "limit-req-rate without rate", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate}}]\n", wantErr: "rate: required"},
		{name: "zero rate", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 0}}]\n", wantErr: "must be positive"},
		{name: "fractional rate", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 1.5}}]\n", wantErr: "whole number"},
		{name: "rate with bad unit", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth, rate: 1q}}]\n", wantErr: "invalid quantity"},
		{name: "rate out of int64 range", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth, rate: 9999P}}]\n", wantErr: "out of range"},
		{name: "plain count rate accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]\n"},
		{name: "decimal human size rate accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]\n"},
		{name: "fractional human size rate accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth, rate: 1.5MB}}]\n"},
		{name: "per-second rate suffix accepted", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth, rate: 1MB/s}}]\n"},

		// Well-formedness only (ADR 0010): no tracker/penalty or
		// reason/penalty coherence checks.
		{name: "penalty not checked against tracker kind", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: rate, leak: 1, volume: 2}\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]\n"},
		{name: "penalty not checked against tracker presence", yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: limit-req-rate, rate: 200}}]\n"},
		{
			name: "shared reason with different penalties accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: tripwire
        actions: [{report: {reason: shared, ttl: 1h, penalty: drop}}]
      - id: flood
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: shared, ttl: 1h, penalty: limit-req-rate, rate: 200}}]
`,
		},

		// The escalation-ladder matrix: a ladder replaces actions and tracker.volume; every refusal names the rule.
		{
			name: "ladder beside actions rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        actions: [stop]
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations: cannot be declared beside actions`,
		},
		{
			name: "ladder beside empty actions rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        actions: []
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations: cannot be declared beside actions`,
		},
		{
			name: "ladder without actions accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [stop]
`,
		},
		{
			name: "empty actions without a ladder accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: []
`,
		},
		{
			name: "ladder beside tracker.volume rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations: cannot be declared beside tracker.volume`,
		},
		{
			name: "ladder without tracker rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations: requires a tracker`,
		},
		{
			name: "single rung ladder rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [stop]
`,
			wantErr: `rule "r": escalations: at least two rungs required`,
		},
		{
			name: "empty ladder rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations: []
`,
			wantErr: `rule "r": escalations: at least two rungs required`,
		},
		{
			name: "equal thresholds rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [stop]
          - at: 5
            actions: [stop]
`,
			wantErr: `rule "r": escalations[1].at: must be strictly ascending`,
		},
		{
			name: "descending thresholds rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 10
            actions: [stop]
          - at: 5
            actions: [stop]
`,
			wantErr: `rule "r": escalations[1].at: must be strictly ascending`,
		},
		{
			name: "byte threshold on rate ladder rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 50MB
            actions: [stop]
          - at: 100
            actions: [stop]
`,
			wantErr: `rule "r": escalations[0].at: invalid quantity`,
		},
		{
			name: "count threshold on rate ladder accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 50
            actions: [stop]
          - at: 100
            actions: [stop]
`,
		},
		{
			name: "human byte threshold on bandwidth ladder accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: bandwidth, leak: 1MB}
        escalations:
          - at: 50MB
            actions: [stop]
          - at: 5GB
            actions: [stop]
`,
		},
		{
			name: "unparseable threshold rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: bandwidth, leak: 1MB}
        escalations:
          - at: 50MB
            actions: [stop]
          - at: bananas
            actions: [stop]
`,
			wantErr: `rule "r": escalations[1].at: invalid quantity`,
		},
		{
			name: "missing threshold rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations[0].at: required`,
		},
		{
			name: "method-shaped rung threshold rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: {method: constant, value: 5}
            actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations[0].at: must be a quantity`,
		},
		{
			name: "replay ladder with plain-number thresholds accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: replay, leak: 1}
        escalations:
          - at: 2
            actions: [stop]
          - at: 5
            actions: [stop]
`,
		},
		{
			name: "byte threshold on replay ladder rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: replay, leak: 1}
        escalations:
          - at: 50MB
            actions: [stop]
          - at: 100
            actions: [stop]
`,
			wantErr: `rule "r": escalations[0].at: invalid quantity`,
		},
		{
			name: "ladder on a replay single-volume rule rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: replay, leak: 1, volume: 5}
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": escalations: cannot be declared beside tracker.volume`,
		},
		{
			name: "rung actions validated by the shared action parser",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{report: {reason: Not A Slug, ttl: 1h, penalty: drop}}]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": action report: reason`,
		},
		{
			name: "rung report ttl validated like a rule's",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{report: {reason: x, ttl: one hour, penalty: drop}}]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": action report: report.ttl`,
		},
		{
			name: "report grace beside ttl rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: 1h, grace: 30m, penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: `rule "r": action report: report.grace: not allowed with report.ttl`,
		},
		{
			name: "rung report grace validated like a rule's",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{report: {reason: x, grace: one hour, penalty: drop}}]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": action report: report.grace`,
		},
		{
			name: "written empty grace refused as a typo",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, grace: "", penalty: limit-req-rate, rate: 200}}]
`,
			wantErr: `rule "r": action report: report.grace: required`,
		},
		{
			name: "rung report penalty validated like a rule's",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{report: {reason: x, ttl: 1h, penalty: ban}}]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": action report: penalty: unknown penalty`,
		},
		{
			name: "rung report rate rules validated like a rule's",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{report: {reason: x, ttl: 1h, penalty: drop, rate: 200}}]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": action report: rate: not allowed`,
		},
		{
			name: "rung limit penalty needs its rate like a rule's",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{report: {reason: x, ttl: 1h, penalty: limit-bandwidth}}]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": action report: rate: required`,
		},
		{
			name: "rung report actions accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: bandwidth, leak: 1MB}
        escalations:
          - at: 50MB
            actions: [{report: {reason: throttle, ttl: 24h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 5GB
            actions: [{report: {reason: ban, ttl: 7d, penalty: drop}}]
`,
		},
		{
			name: "unknown rung action rejected",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [frobnicate]
          - at: 10
            actions: [stop]
`,
			wantErr: `rule "r": unknown action "frobnicate"`,
		},
	}

	// One accepted case per closed vocabulary value (the passing side
	// of "penalty must be one of the five"), generated from the domain's
	// single source (domain.PenaltyActions) so the list cannot drift
	// from the enum. A limit penalty carries a rate.
	for _, action := range domain.PenaltyActions {
		rate := ""
		if action.IsLimit() {
			rate = ", rate: 200"
		}
		tests = append(tests, struct {
			name    string
			yaml    string
			wantErr string
		}{
			name: "penalty " + string(action) + " accepted",
			yaml: "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        actions: [{report: {reason: x, ttl: 1h, penalty: " + string(action) + rate + "}}]\n",
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRules([]byte(tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseRules error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestPlanBannerFollowsThePassedBuild: the plan banner carries the
// header of the build the caller passed - the command constructs the
// per-name build; the plan never names the program itself.
func TestPlanBannerFollowsThePassedBuild(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(validRules), 0o600); err != nil {
		t.Fatal(err)
	}

	build := identity.For("bandage-canary")
	plan, err := Test(cfgPath, rulesPath, build)
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	if !strings.HasPrefix(plan, build.Header()+"\n") {
		t.Errorf("plan does not open with the passed build's header %q:\n%s", build.Header(), plan)
	}
}

// The program-name fallback at the plan seam (ADR 0026): absent a
// reason_prefix, a renamed binary's plan shows its own namespace.
func TestPlanReasonsFollowThePassedBuild(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(validRules), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := Test(cfgPath, rulesPath, identity.For("bandage-canary"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"action report reason=bandage-canary.iso-egress-overflow ttl=168h0m0s grace=none penalty=limit-bandwidth rate=1000000",
		"action report reason=bandage-canary.api-rate-overflow ttl=24h0m0s grace=none penalty=limit-req-rate rate=200",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
}

// The --test plan prints the effective reason prefix - the configured
// reason_prefix, else the passed build's name - and namespaces the
// printed reasons with it; an invalid prefix fails the plan at load,
// naming the key (ADR 0026).
func TestPlanReasonPrefix(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		build   string
		want    []string
		wantErr string
	}{
		{
			name:  "absent key prints the build name as the effective prefix",
			build: "bandage-canary",
			yaml: `version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{report: {reason: iso-egress-overflow, ttl: 1h, penalty: drop}}]
`,
			want: []string{
				"reason prefix: bandage-canary",
				"action report reason=bandage-canary.iso-egress-overflow ttl=1h0m0s grace=none penalty=drop",
			},
		},
		{
			name:  "configured prefix wins and namespaces the printed reasons",
			build: "bandage",
			yaml: `reason_prefix: acme-prod
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{report: {reason: iso-egress-overflow, ttl: 1h, penalty: drop}}]
`,
			want: []string{
				"reason prefix: acme-prod",
				"action report reason=acme-prod.iso-egress-overflow ttl=1h0m0s grace=none penalty=drop",
			},
		},
		{
			name:  "invalid prefix fails the plan naming the key",
			build: "bandage",
			yaml: `reason_prefix: Acme.Prod
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{report: {reason: x, ttl: 1h, penalty: drop}}]
`,
			wantErr: `rules: reason_prefix: "Acme.Prod" is not a valid slug`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "bandage.yml")
			rulesPath := filepath.Join(dir, "rules.yml")
			if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rulesPath, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			plan, err := Test(cfgPath, rulesPath, identity.For(tt.build))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Test succeeded, want error naming the key %q:\n%s", tt.wantErr, plan)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Test error: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(plan, want) {
					t.Errorf("plan does not contain %q:\n%s", want, plan)
				}
			}
		})
	}
}

func TestPrintsValidatedPlan(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(validRules), 0o600); err != nil {
		t.Fatal(err)
	}

	build := identity.For("bandage")
	plan, err := Test(cfgPath, rulesPath, build)
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	// The --test plan output opens with the build identity header,
	// byte for byte.
	if !strings.HasPrefix(plan, build.Header()+"\n") {
		t.Errorf("plan does not open with the identity header %q:\n%s", build.Header(), plan)
	}
	for _, want := range []string{
		"ingest syslog/nginx unixgram /run/bandage/log.sock",
		"db sqlite /var/lib/bandage/reports.db",
		"api 127.0.0.1:8080",
		"cache max_entries 65536",
		"chain main",
		"rule sustained-iso-egress",
		"filter: any",
		"tracker: bandwidth leak=1000000 volume=100000000000 ttl=27h46m40s",
		"action report reason=bandage.iso-egress-overflow ttl=168h0m0s grace=none penalty=limit-bandwidth rate=1000000",
		"chain api",
		"rule bursty-api",
		"tracker: rate leak=10 volume=100 ttl=10s",
		"action report reason=bandage.api-rate-overflow ttl=24h0m0s grace=none penalty=limit-req-rate rate=200",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
}

// Load validation via the plan seam: a bare code passes and prints namespaced;
// a dotted code is refused, quoting the authored value (ADR 0016).
func TestPlanReasonValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
		want    string
	}{
		{
			name: "bare reason code passes and the plan prints it namespaced",
			yaml: `version: 0
chains:
  - name: main
    rules:
      - id: r
        filter: any
        actions: [{report: {reason: iso-egress-overflow, ttl: 1h, penalty: drop}}]
`,
			want: "action report reason=bandage.iso-egress-overflow ttl=1h0m0s grace=none penalty=drop",
		},
		{
			name: "dotted reason code refused quoting the authored value",
			yaml: `version: 0
chains:
  - name: main
    rules:
      - id: r
        filter: any
        actions: [{report: {reason: bandage.x, ttl: 1h, penalty: drop}}]
`,
			wantErr: "bandage.x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "bandage.yml")
			rulesPath := filepath.Join(dir, "rules.yml")
			if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rulesPath, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			plan, err := Test(cfgPath, rulesPath, identity.For("bandage"))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Test succeeded, want error quoting the authored value %q:\n%s", tt.wantErr, plan)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not quote the authored value %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Test error: %v", err)
			}
			if !strings.Contains(plan, tt.want) {
				t.Errorf("plan does not contain %q:\n%s", tt.want, plan)
			}
		})
	}
}

func TestFailsOnBadRules(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Test(cfgPath, rulesPath, identity.For("bandage")); err == nil {
		t.Fatal("Test succeeded, want error")
	}
}

// TestShowsActionsInPlan: the --test plan shows every action of every rule.
func TestShowsActionsInPlan(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(routingRules), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := Test(cfgPath, rulesPath, identity.For("bandage"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"chain main",
		"rule route-downloads",
		"action log level=debug message=\"ROUTE:\"",
		"action jump target=download-abuse",
		"action report reason=bandage.ua-trip ttl=24h0m0s grace=none penalty=drop",
		"action stop",
		"chain download-abuse",
		"rule sustained-iso-egress",
		"action goto target=sink",
		"action report reason=bandage.iso-egress-overflow ttl=168h0m0s grace=none penalty=limit-bandwidth rate=1000000",
		"action stop",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
}

// TestValidatesChainGraph: the --test command rejects unsound goto/jump
// graphs and accepts DAGs.
func TestValidatesChainGraph(t *testing.T) {
	tests := []struct {
		name    string
		rules   string
		wantErr string
	}{
		{
			name: "cyclic rules file rejected",
			rules: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{jump: b}]
  - name: b
    rules:
      - id: s
        actions: [{goto: c}]
  - name: c
    rules:
      - id: t
        actions: [{goto: b}]
`,
			wantErr: "cycle",
		},
		{
			name: "dangling goto target rejected",
			rules: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{goto: nowhere}]
`,
			wantErr: "nonexistent chain",
		},
		{
			name: "dangling rung jump target rejected",
			rules: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{jump: nowhere}]
          - at: 10
            actions: [stop]
`,
			wantErr: "nonexistent chain",
		},
		{
			name: "rung goto cycle rejected",
			rules: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [stop]
          - at: 10
            actions: [{goto: b}]
  - name: b
    rules:
      - id: s
        actions: [{goto: c}]
  - name: c
    rules:
      - id: t
        actions: [{goto: b}]
`,
			wantErr: "cycle",
		},
		{
			name: "document without a main chain rejected",
			rules: `
version: 0
chains:
  - name: a
    rules:
      - id: r
        actions: [stop]
`,
			wantErr: `no chain named "main"`,
		},
		{
			name: "rung goto targeting the entry chain rejected",
			rules: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: b
    rules:
      - id: s
        tracker: {track: rate, leak: 1}
        escalations:
          - at: 5
            actions: [{goto: main}]
          - at: 10
            actions: [stop]
`,
			wantErr: "targets the entry chain",
		},
		{
			name: "duplicate chain names rejected",
			rules: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [stop]
  - name: main
    rules:
      - id: s
        actions: [stop]
`,
			wantErr: "duplicate chain name",
		},
		{
			name:  "acyclic dispatch DAG accepted",
			rules: routingRules,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "bandage.yml")
			rulesPath := filepath.Join(dir, "rules.yml")
			if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rulesPath, []byte(tt.rules), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Test(cfgPath, rulesPath, identity.For("bandage"))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Test error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Test succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

// routingRules is a dispatch DAG (several chains reaching one topic chain)
// exercising the full action vocabulary.
const routingRules = `
version: 0
chains:
  - name: main
    rules:
      - id: route-downloads
        actions:
          - log: "debug,ROUTE:"
          - jump: download-abuse
      - id: ua-trip
        actions: [{report: {reason: ua-trip, ttl: 24h, penalty: drop}}]
      - id: skip-health
        actions: [stop]
  - name: download-abuse
    rules:
      - id: sustained-iso-egress
        tracker: {track: bandwidth, leak: 1MB, volume: 100GB}
        actions:
          - goto: sink
          - report: {reason: iso-egress-overflow, ttl: 7d, penalty: limit-bandwidth, rate: 1MB}
  - name: sink
    rules:
      - id: terminal
        actions: [stop]
`

func TestLoadRulesFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.yml")
	if err := os.WriteFile(path, []byte(validRules), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules error: %v", err)
	}
	if len(doc.Chains) != 2 {
		t.Errorf("chains = %d, want 2", len(doc.Chains))
	}
}

// TestParseRulesFilterErrors: syntax and value errors reject the rules file
// with filter: L:C positions; valid filters select events at the rule level.
func TestParseRulesFilterErrors(t *testing.T) {
	const doc = `version: 0
chains:
  - name: main
    rules:
      - id: r
        filter: %s
`
	tests := []struct {
		name    string
		filter  string // YAML-quoted
		wantErr string
	}{
		{"unknown keyword", `'bogus'`, `rule "r": filter: 1:1: unknown keyword "bogus"`},
		{"trailing token", `'any any'`, `rule "r": filter: 1:5: unexpected "any", expected end of filter`},
		{"missing operand", `'any and'`, `rule "r": filter: 1:8: unexpected end of filter, expected an operand`},
		{"bad src", `'src 10.0.0.0/33'`, `rule "r": filter: 1:5: invalid IP address or prefix "10.0.0.0/33"`},
		{"status value out of range", `'status 20'`, `rule "r": filter: 1:8: status must be within [100, 599]`},
		{"bad size unit spelling", `'sent >= 10MB'`, `rule "r": filter: 1:9: expected a size after "sent", got "10MB"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRules([]byte(fmt.Sprintf(doc, tt.filter)))
			if err == nil {
				t.Fatalf("ParseRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRulesFilterSelectsEvents(t *testing.T) {
	doc, err := ParseRules([]byte(`version: 0
chains:
  - name: main
    rules:
      - id: r
        filter: 'path prefix "/releases" and method GET'
`))
	if err != nil {
		t.Fatalf("ParseRules error: %v", err)
	}
	rule := doc.Chains[0].Rules[0]
	if !rule.Filter.Match(&domain.Event{Method: "GET", Path: "/releases/24.04.iso"}) {
		t.Error("filter does not match a selected event")
	}
	if rule.Filter.Match(&domain.Event{Method: "GET", Path: "/api/data"}) {
		t.Error("filter matches a non-matching path")
	}
	if rule.Filter.Match(&domain.Event{Method: "POST", Path: "/releases/24.04.iso"}) {
		t.Error("filter matches a non-matching method")
	}
}

// TestPrintsFilterSource: the --test plan prints each rule's effective
// filter source expression ("any" where the rule omits it).
func TestPrintsFilterSource(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(`version: 0
chains:
  - name: main
    rules:
      - id: selected
        filter: 'src 10.0.0.0/8 and path prefix "/api"'
      - id: defaulted
`), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := Test(cfgPath, rulesPath, identity.For("bandage"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"rule selected",
		`filter: src 10.0.0.0/8 and path prefix "/api"`,
		"rule defaulted",
		"filter: any",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
}

// TestShowsEnrichActionInPlan: the plan keeps showing each enrich
// action inline in its rule's action list - type always, counter=true
// when the counter is enabled - like every other action, and prints no
// enrichment filter line (the gate is gone with the pre-evaluation
// enrichment path, ADR 0028). The plan contract stays stable for the
// kinds that return (story 15); while the kind vocabulary is empty the
// loader can build no enrich action, so the contract is pinned on
// rules built in code.
func TestShowsEnrichActionInPlan(t *testing.T) {
	rules := Rules{Chains: []domain.Chain{{
		Name: domain.EntryChain,
		Rules: []domain.Rule{
			{
				ID:           "enrich-size",
				FilterSource: "any",
				Actions:      []domain.Action{{Enrich: &domain.EnrichAction{Type: "filesize", Counter: true}}},
			},
			{
				ID:           "iso-ladder",
				FilterSource: "any",
				Tracker:      &domain.TrackerConfig{Kind: domain.TrackBandwidth, Leak: 1e6},
				Escalations: []domain.Escalation{
					{At: 50e6, Actions: []domain.Action{{Enrich: &domain.EnrichAction{Type: "filesize"}}}},
					{At: 5e9, Actions: []domain.Action{{Stop: true}}},
				},
			},
		},
	}}}

	plan := planText(Config{}, rules, identity.For("bandage"))
	for _, want := range []string{
		"rule enrich-size",
		"action enrich type=filesize counter=true",
		"rung at=50000000 enrich type=filesize",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
	if strings.Contains(plan, "enrichment.filter") {
		t.Errorf("plan still prints an enrichment filter line:\n%s", plan)
	}
}

// TestReportsFilterErrorPosition: unparseable filters reject the rules file
// at load, reporting filter: L:C with the rule id.
func TestReportsFilterErrorPosition(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(`version: 0
chains:
  - name: main
    rules:
      - id: typo
        filter: 'status class 9'
`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Test(cfgPath, rulesPath, identity.For("bandage"))
	if err == nil {
		t.Fatal("Test succeeded, want error")
	}
	want := `rule "typo": filter: 1:14: class must be within [1, 5]`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err, want)
	}
}

// TestParseRulesRemovedVolumeMethodBlocksFailThroughQuantityValidation:
// the removed volume-method shapes get no dedicated check (removed
// rule syntax rides generic refusals) - each old block fails load
// through the ordinary quantity validation (a mapping is not a
// quantity), naming the rule (spec: Volume-method removal; the pin
// asserts the generic refusal, with the removed shapes as inputs
// only).
func TestParseRulesRemovedVolumeMethodBlocksFailThroughQuantityValidation(t *testing.T) {
	tests := []struct {
		name    string
		tracker string
		ladder  bool
		wantErr string
	}{
		{name: "constant block on replay", tracker: "track: replay\n          leak: 0.1\n          volume: {method: constant, value: 5}", wantErr: "must be a quantity"},
		{name: "clamped-linear block on replay", tracker: "track: replay\n          leak: 0.1\n          volume: {method: clamped-linear, intercept: 5, slope: -1, min: 2, max: 5}", wantErr: "must be a quantity"},
		{name: "inverse-square block on replay", tracker: "track: replay\n          leak: 0.1\n          volume: {method: inverse-square, base: 1, pivot: 1GB}", wantErr: "must be a quantity"},
		{name: "step block on replay", tracker: "track: replay\n          leak: 0.1\n          volume: {method: step, threshold: 1GB, volume_low: 2, volume_high: 5}", wantErr: "must be a quantity"},
		{name: "block on rate", tracker: "track: rate\n          leak: 1\n          volume: {method: constant, value: 2}", wantErr: "must be a quantity"},
		{name: "block on bandwidth", tracker: "track: bandwidth\n          leak: 1MB\n          volume: {method: step, threshold: 1GB, volume_low: 1, volume_high: 2}", wantErr: "must be a quantity"},
		// Beside a ladder the block rides the escalations-beside-volume
		// refusal instead - still generic, still no dedicated check.
		{name: "block beside a ladder", tracker: "track: replay\n          leak: 1\n          volume: {method: constant, value: 5}", ladder: true, wantErr: "escalations: cannot be declared beside tracker.volume"},
		{name: "empty block", tracker: "track: replay\n          leak: 0.1\n          volume: {}", wantErr: "must be a quantity"},
		{name: "block without a method key", tracker: "track: replay\n          leak: 0.1\n          volume: {value: 2}", wantErr: "must be a quantity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker:\n          " + tt.tracker + "\n"
			if tt.ladder {
				rules += "        escalations:\n          - at: 5\n            actions: [stop]\n          - at: 10\n            actions: [stop]\n"
			} else {
				rules += "        actions: [{report: {reason: x, ttl: 1h, penalty: drop}}]"
			}
			_, err := ParseRules([]byte(rules))
			if err == nil {
				t.Fatalf("ParseRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), `rule "r"`) {
				t.Errorf("error %q does not name the rule", err)
			}
		})
	}
}

// TestParseRulesReplayVolumeScalar: replay's volume is a plain scalar in
// download equivalents - one constant tolerance per rule (spec:
// Volume-method removal).
func TestParseRulesReplayVolumeScalar(t *testing.T) {
	tests := []struct {
		name     string
		volume   string
		wantVol  float64
		wantDesc string // volume description in the --test plan
	}{
		{
			name:     "whole number",
			volume:   "volume: 5",
			wantVol:  5,
			wantDesc: "5",
		},
		{
			name:     "decimal",
			volume:   "volume: 0.1",
			wantVol:  0.1,
			wantDesc: "0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker:\n          track: replay\n          leak: 0.1\n          " + tt.volume
			doc, err := ParseRules([]byte(yaml))
			if err != nil {
				t.Fatalf("ParseRules error: %v", err)
			}
			cfg := doc.Chains[0].Rules[0].Tracker
			if cfg == nil || cfg.Kind != domain.TrackReplay {
				t.Fatalf("tracker = %+v, want replay", cfg)
			}
			if cfg.Leak != 0.1 {
				t.Errorf("leak = %v, want 0.1", cfg.Leak)
			}
			if cfg.Volume != tt.wantVol {
				t.Errorf("volume = %v, want %v", cfg.Volume, tt.wantVol)
			}
			if got := cfg.VolumeDesc(); got != tt.wantDesc {
				t.Errorf("volume description = %q, want %q", got, tt.wantDesc)
			}
			if cfg.TTL != 0 {
				t.Errorf("ttl = %v, want 0 (replay derives its TTL per event)", cfg.TTL)
			}
			if cfg.TargetPrefix != domain.DefaultTargetPrefix {
				t.Errorf("target_prefix = %+v, want default", cfg.TargetPrefix)
			}
		})
	}
}

// TestParseRulesVolumeValidation: each tracker kind's volume parses in
// its own quantity language - plain counts for rate and replay, byte
// quantities for bandwidth - and must be a positive scalar.
func TestParseRulesVolumeValidation(t *testing.T) {
	tests := []struct {
		name    string
		tracker string
		wantErr string
	}{
		{
			name:    "non-scalar volume on rate rejected",
			tracker: "track: rate\n          leak: 1\n          volume: [1, 2]",
			wantErr: "must be a quantity",
		},
		{
			name:    "non-scalar volume on bandwidth rejected",
			tracker: "track: bandwidth\n          leak: 1MB\n          volume: [1MB]",
			wantErr: "must be a quantity",
		},
		{
			name:    "rate scalar volume accepted",
			tracker: "track: rate\n          leak: 1\n          volume: 2",
		},
		{
			name:    "bandwidth byte volume accepted",
			tracker: "track: bandwidth\n          leak: 1MB/s\n          volume: 100GB",
		},
		{
			name:    "replay scalar volume accepted",
			tracker: "track: replay\n          leak: 0.1\n          volume: 5",
		},
		{
			name:    "replay byte quantity volume rejected",
			tracker: "track: replay\n          leak: 0.1\n          volume: 1GB",
			wantErr: "tracker.volume: invalid quantity",
		},
		{
			name:    "replay byte quantity leak rejected",
			tracker: "track: replay\n          leak: 1MB/s\n          volume: 5",
			wantErr: "tracker.leak: invalid quantity",
		},
		{
			name:    "replay zero scalar volume rejected",
			tracker: "track: replay\n          leak: 0.1\n          volume: 0",
			wantErr: "tracker.volume: must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker:\n          " + tt.tracker + "\n        actions: [{report: {reason: x, ttl: 1h, penalty: drop}}]"
			_, err := ParseRules([]byte(yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseRules error: %v, want success", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseRules succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), `rule "r"`) {
				t.Errorf("error %q does not name the rule", err)
			}
		})
	}
}

// TestShowsReplayVolumeScalar: the --test plan shows the replay tracker's
// plain scalar volume and its per-event TTL derivation.
func TestShowsReplayVolumeScalar(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(`
version: 0
chains:
  - name: main
    rules:
      - id: iso-replay
        tracker: {track: replay, leak: 0.1, volume: 5}
        actions: [{report: {reason: iso-replay, ttl: 24h, penalty: limit-bandwidth, rate: 1MB}}]
      - id: small-replay
        tracker: {track: replay, leak: 0, volume: 0.1}
        actions: [{report: {reason: small-replay, ttl: 24h, penalty: limit-bandwidth, rate: 1MB}}]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := Test(cfgPath, rulesPath, identity.For("bandage"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"tracker: replay leak=0.1 volume=5 ttl=per-event",
		"tracker: replay leak=0 volume=0.1 ttl=per-event",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
}

const ladderRules = `
version: 0
chains:
  - name: main
    rules:
      - id: iso-ladder
        tracker: {track: bandwidth, leak: 1MB}
        escalations:
          - at: 50MB
            actions: [{jump: inspect}]
          - at: 5GB
            actions: [{report: {reason: iso-throttle, ttl: 24h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 40GB
            actions:
              - log: "debug,MARK:TOP"
              - report: {reason: iso-ban, ttl: 7d, penalty: drop}
  - name: inspect
    rules:
      - id: watch
        actions: [{log: "debug,MARK:WATCH"}]
`

// A ladder carries no action list and no tracker.volume; the cache-entry TTL derives from the top rung.
func TestParseRulesEscalations(t *testing.T) {
	doc, err := ParseRules([]byte(ladderRules))
	if err != nil {
		t.Fatalf("ParseRules error: %v", err)
	}
	chains := doc.Chains
	iso := chains[0].Rules[0]
	if len(iso.Actions) != 0 {
		t.Errorf("rule actions = %+v, want none (the ladder carries the actions)", iso.Actions)
	}
	if iso.Tracker == nil || iso.Tracker.Kind != domain.TrackBandwidth || iso.Tracker.Leak != 1e6 {
		t.Fatalf("tracker = %+v, want bandwidth leak 1e6", iso.Tracker)
	}
	if iso.Tracker.Volume != 0 {
		t.Errorf("tracker volume = %v, want 0 (the ladder's rungs stand in for it)", iso.Tracker.Volume)
	}
	if iso.Tracker.TTL != 40000*time.Second {
		t.Errorf("tracker TTL = %v, want %v (top rung 40GB / leak 1MB)", iso.Tracker.TTL, 40000*time.Second)
	}

	wantAt := []float64{5e7, 5e9, 4e10}
	if len(iso.Escalations) != len(wantAt) {
		t.Fatalf("escalations = %+v, want %d rungs", iso.Escalations, len(wantAt))
	}
	for i, at := range wantAt {
		if iso.Escalations[i].At != at {
			t.Errorf("rung %d at = %v, want %v", i, iso.Escalations[i].At, at)
		}
	}
	r0 := iso.Escalations[0].Actions
	if len(r0) != 1 || r0[0].Jump != "inspect" {
		t.Errorf("rung 0 actions = %+v, want one jump to inspect", r0)
	}
	r1 := iso.Escalations[1].Actions
	if len(r1) != 1 || r1[0].Report == nil || r1[0].Report.Reason != "iso-throttle" {
		t.Errorf("rung 1 actions = %+v, want one report iso-throttle", r1)
	}
	r2 := iso.Escalations[2].Actions
	if len(r2) != 2 || r2[0].Log == nil || r2[1].Report == nil || r2[1].Report.Reason != "iso-ban" {
		t.Errorf("rung 2 actions = %+v, want log then report iso-ban", r2)
	}
	if p := r2[1].Report.Penalty; p.Action != domain.PenaltyDrop || p.Rate != nil {
		t.Errorf("rung 2 penalty = %+v, want drop without rate", p)
	}
}

// The TTL keeps the zero-leak default and the one-second minimum of the single-volume derivation.
func TestParseRulesEscalationTTLDefaults(t *testing.T) {
	tests := []struct {
		name string
		leak string
		at   string
		at2  string
		kind string
		want time.Duration
	}{
		{name: "zero leak falls back to the default TTL", kind: "rate", leak: "0", at: "5", at2: "10", want: domain.DefaultTTL},
		{name: "top rung over leak, minimum kept", kind: "rate", leak: "10", at: "5", at2: "5.5", want: domain.MinTTL},
		{name: "top rung over leak", kind: "rate", leak: "2", at: "5", at2: "20", want: 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "version: 0\nchains:\n  - name: main\n    rules:\n      - id: r\n        tracker: {track: " + tt.kind + ", leak: " + tt.leak + "}\n" +
				"        escalations:\n          - at: " + tt.at + "\n            actions: [stop]\n          - at: " + tt.at2 + "\n            actions: [stop]"
			doc, err := ParseRules([]byte(yaml))
			if err != nil {
				t.Fatalf("ParseRules error: %v", err)
			}
			if got := doc.Chains[0].Rules[0].Tracker.TTL; got != tt.want {
				t.Errorf("tracker TTL = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShowsEscalationRungsInPlan(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte(ladderRules), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := Test(cfgPath, rulesPath, identity.For("bandage"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"tracker: bandwidth leak=1000000 ttl=11h6m40s",
		"rung at=50000000 jump target=inspect",
		"rung at=5000000000 report reason=bandage.iso-throttle ttl=24h0m0s grace=none penalty=limit-bandwidth rate=1000000",
		`rung at=40000000000 log level=debug message="MARK:TOP", report reason=bandage.iso-ban ttl=168h0m0s grace=none penalty=drop`,
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
	if strings.Contains(plan, "tracker: bandwidth leak=1000000 volume=") {
		t.Errorf("ladder tracker line still prints volume:\n%s", plan)
	}
}

// Report lifecycle validation (spec: Load validation additions): a
// report action without ttl declares a dynamic report and needs the
// tracker liveness to die by - a tracker-less rule or a zero-leak
// tracker can never underflow - and every refusal names the rule and
// the report action's reason.
func TestRefusesDynamicReportsWithoutLiveness(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErrs []string // empty: accepted
	}{
		{
			name: "ttl-less report on a tracked rule accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, penalty: limit-req-rate, rate: 200}}]
`,
		},
		{
			name: "ttl-less report on a tracker-less rule refused",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        actions: [{report: {reason: x, penalty: drop}}]
`,
			wantErrs: []string{`rule "r"`, `action report "x"`, "report.ttl", "no tracker"},
		},
		{
			name: "ttl-less report on a zero-leak tracker refused",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 0, volume: 2}
        actions: [{report: {reason: x, penalty: limit-req-rate, rate: 200}}]
`,
			wantErrs: []string{`rule "r"`, `action report "x"`, "report.ttl", "zero-leak"},
		},
		{
			name: "ttl-less rung report on a zero-leak ladder refused",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 0}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, ttl: 1h, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, penalty: drop}}]
`,
			wantErrs: []string{`rule "ladder"`, `action report "ban"`, "zero-leak"},
		},
		{
			name: "ttl-less rung report on a leaking ladder accepted",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: ladder
        tracker: {track: bandwidth, leak: 1}
        escalations:
          - at: 50
            actions: [{report: {reason: throttle, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 150
            actions: [{report: {reason: ban, penalty: drop}}]
`,
		},
		{
			name: "written empty ttl refused as a typo",
			yaml: `
version: 0
chains:
  - name: main
    rules:
      - id: r
        tracker: {track: rate, leak: 1, volume: 2}
        actions: [{report: {reason: x, ttl: "", penalty: limit-req-rate, rate: 200}}]
`,
			wantErrs: []string{`rule "r"`, "report.ttl", "required"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRules([]byte(tt.yaml))
			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("ParseRules error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ParseRules succeeded, want the report action refused")
			}
			for _, want := range tt.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
		})
	}
}

// TestShowsReportLifecyclesInPlan: the --test plan prints the
// lifecycle of every report action - the fixed report's ttl,
// ttl=none for a dynamic one, and grace= beside ttl= on every
// report line (spec: Load validation additions, Report action
// grammar).
func TestShowsReportLifecyclesInPlan(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bandage.yml")
	rulesPath := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(cfgPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	rules := `version: 0
chains:
  - name: main
    rules:
      - id: dyn
        tracker: {track: rate, leak: 1, volume: 10}
        actions: [{report: {reason: dyn-overflow, penalty: limit-req-rate, rate: 200}}]
      - id: graced
        tracker: {track: rate, leak: 1, volume: 10}
        actions: [{report: {reason: graced-overflow, grace: 1d, penalty: limit-req-rate, rate: 200}}]
      - id: fixed
        tracker: {track: rate, leak: 1, volume: 10}
        actions: [{report: {reason: fixed-overflow, ttl: 24h, penalty: limit-req-rate, rate: 200}}]
      - id: ladder
        tracker: {track: bandwidth, leak: 1MB/s}
        escalations:
          - at: 10MB
            actions: [{report: {reason: throttle, penalty: limit-bandwidth, rate: 1MB}}]
          - at: 100MB
            actions: [{report: {reason: ban, ttl: 7d, penalty: drop}}]
`
	if err := os.WriteFile(rulesPath, []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := Test(cfgPath, rulesPath, identity.For("bandage"))
	if err != nil {
		t.Fatalf("Test error: %v", err)
	}
	for _, want := range []string{
		"action report reason=bandage.dyn-overflow ttl=none grace=none penalty=limit-req-rate rate=200",
		"action report reason=bandage.graced-overflow ttl=none grace=24h0m0s penalty=limit-req-rate rate=200",
		"action report reason=bandage.fixed-overflow ttl=24h0m0s grace=none penalty=limit-req-rate rate=200",
		"rung at=10000000 report reason=bandage.throttle ttl=none grace=none penalty=limit-bandwidth rate=1000000",
		"rung at=100000000 report reason=bandage.ban ttl=168h0m0s grace=none penalty=drop",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not contain %q:\n%s", want, plan)
		}
	}
}
