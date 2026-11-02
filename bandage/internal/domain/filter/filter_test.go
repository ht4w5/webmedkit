package filter

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// baseEvent is the event the table cases vary; zero fields never match.
func baseEvent() domain.Event {
	return domain.Event{
		RemoteAddr: mustAddr("203.0.113.7"),
		Method:     "GET",
		Path:       "/releases/24.04.iso",
		Agent:      "curl/8.5.0",
		Status:     200,
		SentBytes:  1024,
	}
}

func mustAddr(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return a
}

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(err)
	}
	return p
}

func TestFilterMatch(t *testing.T) {
	tests := []struct {
		name string
		src  string
		mut  func(*domain.Event) // applied to baseEvent
		want bool
	}{
		// any / none.
		{"any matches", "any", nil, true},
		{"none does not match", "none", nil, false},
		{"not none matches", "not none", nil, true},

		// src: exact address or prefix containment.
		{"src exact address", "src 203.0.113.7", nil, true},
		{"src exact address mismatch", "src 203.0.113.8", nil, false},
		{"src prefix contains", "src 203.0.113.0/24", nil, true},
		{"src prefix excludes", "src 198.51.100.0/24", nil, false},
		{"src single-address prefix", "src 203.0.113.7/32", nil, true},
		{"src ipv6 exact", "src 2001:db8::1", func(e *domain.Event) {
			e.RemoteAddr = mustAddr("2001:db8::1")
		}, true},
		{"src ipv6 prefix", "src 2001:db8::/64", func(e *domain.Event) {
			e.RemoteAddr = mustAddr("2001:db8::42")
		}, true},
		{"src ipv4 does not match ipv6", "src 203.0.113.7", func(e *domain.Event) {
			e.RemoteAddr = mustAddr("2001:db8::1")
		}, false},

		// method: exact and case-sensitive.
		{"method exact", "method GET", nil, true},
		{"method mismatch", "method POST", nil, false},
		{"method case-sensitive", "method get", nil, false},

		// path: exact / prefix / suffix / keyword / regex, case-sensitive.
		{"path exact", `path "/releases/24.04.iso"`, nil, true},
		{"path exact mismatch", `path "/releases"`, nil, false},
		{"path prefix match", `path prefix "/releases"`, nil, true},
		{"path prefix mismatch", `path prefix "/api"`, nil, false},
		{"path prefix case-sensitive", `path prefix "/RELEASES"`, nil, false},
		{"path suffix match", `path suffix ".iso"`, nil, true},
		{"path suffix mismatch", `path suffix ".img"`, nil, false},
		{"path keyword match", `path keyword "24.04"`, nil, true},
		{"path keyword mismatch", `path keyword "22.04"`, nil, false},
		{"path regex match", `path regex "\\.iso$"`, nil, true},
		{"path regex unanchored", `path regex "releases"`, nil, true},
		{"path regex mismatch", `path regex "\\.deb$"`, nil, false},
		{"path regex anchors respected", `path regex "^24\\.04"`, nil, false},
		{"path regex case-sensitive", `path regex "RELEASES"`, nil, false},
		{"path regex case-insensitive flag", `path regex "(?i)RELEASES"`, nil, true},

		// agent: exact / prefix / suffix / keyword / regex, case-sensitive.
		{"agent exact", `agent "curl/8.5.0"`, nil, true},
		{"agent exact mismatch", `agent "curl"`, nil, false},
		{"agent exact case-sensitive", `agent "CURL/8.5.0"`, nil, false},
		{"agent prefix match", `agent prefix "curl/"`, nil, true},
		{"agent prefix mismatch", `agent prefix "wget/"`, nil, false},
		{"agent prefix case-sensitive", `agent prefix "CURL/"`, nil, false},
		{"agent suffix match", `agent suffix ".0"`, nil, true},
		{"agent suffix mismatch", `agent suffix ".img"`, nil, false},
		{"agent suffix case-sensitive", `agent suffix "T/1.0"`, func(e *domain.Event) { e.Agent = "Wget/1.0" }, false},
		{"agent keyword match", `agent keyword "curl"`, nil, true},
		{"agent keyword mismatch", `agent keyword "wget"`, nil, false},
		{"agent regex match", `agent regex "curl"`, nil, true},
		{"agent regex alternation", `agent regex "curl|wget"`, nil, true},
		{"agent regex mismatch", `agent regex "bot"`, nil, false},
		{"agent regex case-sensitive", `agent regex "CURL"`, nil, false},
		{"agent regex case-insensitive flag", `agent regex "(?i)CURL"`, nil, true},

		// single-quoted strings: interchangeable with the double-quoted spelling
		// at every text value; '' is the only escape (one literal quote), every
		// other character - backslashes included - is literal.
		{"path exact single-quoted equivalence", `path '/releases/24.04.iso'`, nil, true},
		{"path exact single-quoted mismatch", `path '/releases'`, nil, false},
		{"agent prefix single-quoted equivalence", `agent prefix 'curl/'`, nil, true},
		{"agent prefix single-quoted mismatch", `agent prefix 'wget/'`, nil, false},
		{"agent suffix single-quoted equivalence", `agent suffix '.0'`, nil, true},
		{"path regex single-quoted pairs with double-quoted", `path regex '\.iso$'`, nil, true},
		{"path regex single-quoted mismatch", `path regex '\.deb$'`, nil, false},
		{"path regex single-quoted unanchored", `path regex 'releases'`, nil, true},
		{"path regex single-quoted case-sensitive", `path regex 'RELEASES'`, nil, false},
		{"path regex single-quoted case-insensitive flag", `path regex '(?i)RELEASES'`, nil, true},
		{"single-quoted backslashes are literal", `path '/releases/24\.04.iso'`, func(e *domain.Event) { e.Path = `/releases/24\.04.iso` }, true},
		{"single-quoted backslash text is not an escape", `path keyword '\.iso'`, nil, false},
		{"single-quoted values compose with combinators", `not path suffix '\.img' and (agent prefix 'curl/' or agent prefix 'wget/')`, nil, true},
		{"single-quoted doubled quote is one literal quote", `agent suffix 'don''t'`, func(e *domain.Event) { e.Agent = "curl/don't" }, true},

		// status: code or class.
		{"status exact", "status 200", nil, true},
		{"status mismatch", "status 404", nil, false},
		{"status class 2", "status class 2", nil, true},
		{"status class 3 on 200", "status class 3", nil, false},
		{"status class 4 on 404", "status class 4", func(e *domain.Event) { e.Status = 404 }, true},
		{"status class 5 on 599", "status class 5", func(e *domain.Event) { e.Status = 599 }, true},
		{"status class 1 on 100", "status class 1", func(e *domain.Event) { e.Status = 100 }, true},

		// sent: >= / <= with binary units, inclusive boundaries.
		{"sent gte inclusive boundary", "sent >= 1024", nil, true},
		{"sent gte above", "sent >= 1023", nil, true},
		{"sent gte below", "sent >= 1025", nil, false},
		{"sent lte inclusive boundary", "sent <= 1024", nil, true},
		{"sent lte below", "sent <= 1025", nil, true},
		{"sent lte above", "sent <= 1023", nil, false},
		{"sent k is binary", "sent >= 1k", func(e *domain.Event) { e.SentBytes = 1024 }, true},
		{"sent k decimal does not match", "sent >= 1k", func(e *domain.Event) { e.SentBytes = 1000 }, false},
		{"sent m is binary", "sent <= 1m", func(e *domain.Event) { e.SentBytes = 1 << 20 }, true},
		{"sent g is binary", "sent >= 1g", func(e *domain.Event) { e.SentBytes = 1 << 30 }, true},
		{"sent zero limit", "sent >= 0", nil, true},

		// size: >= / <= over the Filesize estimate, inclusive at the boundary.
		// The base event's Filesize is unknown; these set a known one.
		{"size gte inclusive boundary", "size >= 1024", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"size gte above", "size >= 1023", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"size gte below", "size >= 1025", func(e *domain.Event) { e.SizeBytes = 1024 }, false},
		{"size lte inclusive boundary", "size <= 1024", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"size lte below", "size <= 1025", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"size lte above", "size <= 1023", func(e *domain.Event) { e.SizeBytes = 1024 }, false},
		{"size k is binary", "size >= 1k", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"size k decimal does not match", "size >= 1k", func(e *domain.Event) { e.SizeBytes = 1000 }, false},
		{"size m is binary", "size <= 1m", func(e *domain.Event) { e.SizeBytes = 1 << 20 }, true},
		{"size g is binary", "size >= 1g", func(e *domain.Event) { e.SizeBytes = 1 << 30 }, true},

		// A comparison claims only events whose Filesize is known (size 0 means unknown).
		{"size gte skips unknown", "size >= 0", nil, false},
		{"size lte skips unknown", "size <= 1g", nil, false},
		{"size gte zero is known-size", "size >= 0", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"size lte zero matches nothing", "size <= 0", func(e *domain.Event) { e.SizeBytes = 1024 }, false},

		// size unknown: exactly the events whose Filesize is unknown (size 0 means unknown).
		{"size unknown matches unknown", "size unknown", nil, true},
		{"size unknown skips known", "size unknown", func(e *domain.Event) { e.SizeBytes = 1024 }, false},
		{"size unknown skips minimal known size", "size unknown", func(e *domain.Event) { e.SizeBytes = 1 }, false},
		{"not size unknown is the complement", "not size unknown", nil, false},
		{"not size unknown matches known", "not size unknown", func(e *domain.Event) { e.SizeBytes = 1024 }, true},

		// Partition pinned: comparisons never claim unknowns, >= 0 is "known", <= 0 matches nothing.
		{"size gte zero matches minimal known size", "size >= 0", func(e *domain.Event) { e.SizeBytes = 1 }, true},
		{"size lte zero matches nothing even at minimal size", "size <= 0", func(e *domain.Event) { e.SizeBytes = 1 }, false},
		{"size unknown or known covers unknown", "size unknown or size >= 0", nil, true},
		{"size unknown or known covers known", "size unknown or size >= 0", func(e *domain.Event) { e.SizeBytes = 1 << 30 }, true},
		{"size unknown and known never overlap on unknown", "size unknown and size >= 0", nil, false},
		{"size unknown and known never overlap on known", "size unknown and size >= 0", func(e *domain.Event) { e.SizeBytes = 1024 }, false},

		// size unknown composes like every predicate.
		{"size unknown and match", "size unknown and method GET", nil, true},
		{"size unknown and mismatch", "size unknown and method POST", nil, false},
		{"size unknown or comparison", "size unknown or size >= 1k", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"not size unknown and comparison", "not size unknown and size <= 1k", func(e *domain.Event) { e.SizeBytes = 1024 }, true},
		{"parens with size unknown", "(size unknown or size >= 1g) and method GET", nil, true},
		{"parens change size unknown result", "(size unknown or size >= 1g) and method POST", nil, false},
		{"symbol combinators with size unknown", "size unknown && ! status 404", nil, true},

		// combinators: precedence not > and > or, parentheses override.
		{"and all match", `method GET and status 200`, nil, true},
		{"and one mismatch", `method GET and status 404`, nil, false},
		{"or one match", `method POST or status 200`, nil, true},
		{"or none match", `method POST or status 404`, nil, false},
		{"not mismatch matches", `not status 404`, nil, true},
		{"not match does not match", `not status 200`, nil, false},
		{"and binds tighter than or", `method POST or status 200 and method GET`, nil, true},
		{"and-or precedence excludes", `method POST or status 404 and method GET`, nil, false},
		{"parens override precedence", `(method POST or status 200) and method GET`, nil, true},
		{"parens change result", `(method POST or status 404) and method GET`, nil, false},
		{"not binds tighter than and", `not status 404 and method GET`, nil, true},
		{"not-and precedence", `not (status 404 and method GET)`, nil, true},

		// Symbols are spelled words.
		{"symbol and", `method GET && status 200`, nil, true},
		{"symbol or", `method POST || status 200`, nil, true},
		{"symbol not", `! status 404`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(tt.src)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.src, err)
			}
			ev := baseEvent()
			if tt.mut != nil {
				tt.mut(&ev)
			}
			if got := f.Match(&ev); got != tt.want {
				t.Errorf("Parse(%q).Match(%+v) = %v, want %v", tt.src, ev, got, tt.want)
			}
		})
	}
}

// The constructors validate their arguments.
func TestConstructorValidation(t *testing.T) {
	any, none := &FilterAny{}, &FilterNone{}
	validPrefix := mustPrefix("10.0.0.0/8")
	tests := []struct {
		name string
		err  error
	}{
		{"and empty", func() error { _, err := NewFilterAnd(); return err }()},
		{"and nil member", func() error { _, err := NewFilterAnd(any, nil); return err }()},
		{"or empty", func() error { _, err := NewFilterOr(); return err }()},
		{"or nil member", func() error { _, err := NewFilterOr(nil, none); return err }()},
		{"not nil", func() error { _, err := NewFilterNot(nil); return err }()},
		{"invalid prefix", func() error { _, err := NewFilterRemoteAddr(netip.Prefix{}); return err }()},
		{"empty method", func() error { _, err := NewFilterMethod(""); return err }()},
		{"empty path", func() error { _, err := NewFilterPath(""); return err }()},
		{"empty path prefix", func() error { _, err := NewFilterPathPrefix(""); return err }()},
		{"empty path suffix", func() error { _, err := NewFilterPathSuffix(""); return err }()},
		{"empty path keyword", func() error { _, err := NewFilterPathKeyword(""); return err }()},
		{"empty path pattern", func() error { _, err := NewFilterPathRegex(""); return err }()},
		{"invalid path pattern", func() error { _, err := NewFilterPathRegex("("); return err }()},
		{"empty agent", func() error { _, err := NewFilterAgent(""); return err }()},
		{"empty agent prefix", func() error { _, err := NewFilterAgentPrefix(""); return err }()},
		{"empty agent suffix", func() error { _, err := NewFilterAgentSuffix(""); return err }()},
		{"empty agent keyword", func() error { _, err := NewFilterAgentKeyword(""); return err }()},
		{"empty agent pattern", func() error { _, err := NewFilterAgentRegex(""); return err }()},
		{"invalid agent pattern", func() error { _, err := NewFilterAgentRegex("["); return err }()},
		{"status below range", func() error { _, err := NewFilterStatus(99); return err }()},
		{"status above range", func() error { _, err := NewFilterStatus(600); return err }()},
		{"class below range", func() error { _, err := NewFilterStatusClass(0); return err }()},
		{"class above range", func() error { _, err := NewFilterStatusClass(6); return err }()},
		{"sent at least negative", func() error { _, err := NewFilterSentAtLeast(-1); return err }()},
		{"sent at most negative", func() error { _, err := NewFilterSentAtMost(-1); return err }()},
		{"size at least negative", func() error { _, err := NewFilterSizeAtLeast(-1); return err }()},
		{"size at most negative", func() error { _, err := NewFilterSizeAtMost(-1); return err }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("constructor succeeded, want error")
			}
		})
	}
	// Valid constructions succeed.
	if _, err := NewFilterAnd(any, none); err != nil {
		t.Errorf("NewFilterAnd error: %v", err)
	}
	if _, err := NewFilterOr(any, none); err != nil {
		t.Errorf("NewFilterOr error: %v", err)
	}
	if _, err := NewFilterNot(none); err != nil {
		t.Errorf("NewFilterNot error: %v", err)
	}
	if _, err := NewFilterRemoteAddr(validPrefix); err != nil {
		t.Errorf("NewFilterRemoteAddr error: %v", err)
	}
	if _, err := NewFilterPathRegex(`\.iso$`); err != nil {
		t.Errorf("NewFilterPathRegex error: %v", err)
	}
	if _, err := NewFilterAgentPrefix("curl/"); err != nil {
		t.Errorf("NewFilterAgentPrefix error: %v", err)
	}
	if _, err := NewFilterAgentSuffix(".0"); err != nil {
		t.Errorf("NewFilterAgentSuffix error: %v", err)
	}
	if _, err := NewFilterAgentRegex(`(?i)curl`); err != nil {
		t.Errorf("NewFilterAgentRegex error: %v", err)
	}
	if _, err := NewFilterSizeAtLeast(0); err != nil {
		t.Errorf("NewFilterSizeAtLeast error: %v", err)
	}
	if _, err := NewFilterSizeAtMost(0); err != nil {
		t.Errorf("NewFilterSizeAtMost error: %v", err)
	}
}

// ParseError stays a *ParseError through the public API (errors.As).
func TestParseErrorType(t *testing.T) {
	_, err := Parse("bogus")
	if err == nil {
		t.Fatal("Parse succeeded, want error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a *ParseError", err)
	}
}
