package filter

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// treeString renders a parsed filter as a canonical s-expression so parse trees
// (precedence, n-ary flattening) are assertable.
func treeString(f Filter) string {
	switch v := f.(type) {
	case *FilterAny:
		return "any"
	case *FilterNone:
		return "none"
	case *FilterAnd:
		return "and(" + treeList(v.filters) + ")"
	case *FilterOr:
		return "or(" + treeList(v.filters) + ")"
	case *FilterNot:
		return "not(" + treeString(v.filter) + ")"
	case *FilterRemoteAddr:
		return "src " + v.prefix.String()
	case *FilterMethod:
		return "method " + v.method
	case *FilterPath:
		return `path "` + v.path + `"`
	case *FilterPathPrefix:
		return `path prefix "` + v.prefix + `"`
	case *FilterPathSuffix:
		return `path suffix "` + v.suffix + `"`
	case *FilterPathKeyword:
		return `path keyword "` + v.keyword + `"`
	case *FilterPathRegex:
		return `path regex "` + v.re.String() + `"`
	case *FilterAgent:
		return `agent "` + v.agent + `"`
	case *FilterAgentPrefix:
		return `agent prefix "` + v.prefix + `"`
	case *FilterAgentSuffix:
		return `agent suffix "` + v.suffix + `"`
	case *FilterAgentKeyword:
		return `agent keyword "` + v.keyword + `"`
	case *FilterAgentRegex:
		return `agent regex "` + v.re.String() + `"`
	case *FilterStatus:
		return "status " + itoa(v.status)
	case *FilterStatusClass:
		return "status class " + itoa(v.class)
	case *FilterSentAtLeast:
		return "sent >= " + ltoa(v.limit)
	case *FilterSentAtMost:
		return "sent <= " + ltoa(v.limit)
	case *FilterSizeAtLeast:
		return "size >= " + ltoa(v.limit)
	case *FilterSizeAtMost:
		return "size <= " + ltoa(v.limit)
	case *FilterSizeUnknown:
		return "size unknown"
	default:
		return "?"
	}
}

func treeList(filters []Filter) string {
	parts := make([]string, len(filters))
	for i, f := range filters {
		parts[i] = treeString(f)
	}
	return strings.Join(parts, ", ")
}

func itoa(n int) string   { return strconv.Itoa(n) }
func ltoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestParseTrees(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		// Primitives.
		{"any", "any", "any"},
		{"none", "none", "none"},
		{"src address becomes full-length prefix", "src 10.0.0.1", "src 10.0.0.1/32"},
		{"src prefix", "src 10.0.0.0/8", "src 10.0.0.0/8"},
		{"src ipv6 address", "src 2001:db8::1", "src 2001:db8::1/128"},
		{"src ipv6 prefix", "src 2001:db8::/64", "src 2001:db8::/64"},
		{"method", "method GET", "method GET"},
		{"path exact", `path "/x"`, `path "/x"`},
		{"path prefix", `path prefix "/x"`, `path prefix "/x"`},
		{"path suffix", `path suffix ".iso"`, `path suffix ".iso"`},
		{"path keyword", `path keyword "iso"`, `path keyword "iso"`},
		{"path regex", `path regex "\\.iso$"`, `path regex "\.iso$"`},
		{"path regex flag passes through", `path regex "(?i)\\.iso$"`, `path regex "(?i)\.iso$"`},
		{"agent exact", `agent "curl"`, `agent "curl"`},
		{"agent prefix", `agent prefix "curl/"`, `agent prefix "curl/"`},
		{"agent suffix", `agent suffix ".0"`, `agent suffix ".0"`},
		{"agent keyword", `agent keyword "curl"`, `agent keyword "curl"`},
		{"agent regex", `agent regex "(?i)curl"`, `agent regex "(?i)curl"`},
		{"status code", "status 404", "status 404"},
		{"status class", "status class 5", "status class 5"},
		{"sent gte bare", "sent >= 1024", "sent >= 1024"},
		{"sent lte bare", "sent <= 0", "sent <= 0"},
		{"sent binary k", "sent >= 10k", "sent >= 10240"},
		{"sent binary m case-insensitive", "sent >= 10M", "sent >= 10485760"},
		{"sent binary g", "sent >= 2g", "sent >= 2147483648"},
		{"size gte bare", "size >= 1024", "size >= 1024"},
		{"size lte bare", "size <= 0", "size <= 0"},
		{"size binary k", "size >= 10k", "size >= 10240"},
		{"size binary m case-insensitive", "size >= 10M", "size >= 10485760"},
		{"size binary g", "size >= 2g", "size >= 2147483648"},
		{"size unknown", "size unknown", "size unknown"},
		{"size unknown in combinators", "not size unknown and size >= 0", "and(not(size unknown), size >= 0)"},

		// String escapes unescape to the matched value.
		{"string escapes", `path "a\"b\\c\nd\te"`, "path \"a\"b\\c\nd\te\""},

		// Single-quoted strings produce the same tokens as the double-quoted
		// spelling at every text value; backslash sequences stay literal.
		{"single-quoted path exact", `path '/x'`, `path "/x"`},
		{"single-quoted path prefix", `path prefix '/x'`, `path prefix "/x"`},
		{"single-quoted path suffix", `path suffix '.iso'`, `path suffix ".iso"`},
		{"single-quoted path keyword", `path keyword 'iso'`, `path keyword "iso"`},
		{"single-quoted path regex", `path regex '\.iso$'`, `path regex "\.iso$"`},
		{"single-quoted agent exact", `agent 'curl'`, `agent "curl"`},
		{"single-quoted agent prefix", `agent prefix 'curl/'`, `agent prefix "curl/"`},
		{"single-quoted agent suffix", `agent suffix '.0'`, `agent suffix ".0"`},
		{"single-quoted agent keyword", `agent keyword 'curl'`, `agent keyword "curl"`},
		{"single-quoted agent regex", `agent regex '(?i)curl'`, `agent regex "(?i)curl"`},
		{"single-quoted doubled quote is one quote", `path 'a''b'`, `path "a'b"`},
		{"single-quoted string ends at unpaired quote", `path 'foo\'`, `path "foo\"`},
		{"single-quoted '''' is one literal quote", `path ''''`, `path "'"`},
		{"single-quoted '''''' is two literal quotes", `path ''''''`, `path "''"`},
		{"single-quoted backslash sequences are literal", `path 'a\.b'`, `path "a\.b"`},
		{"single-quoted string spans newlines", "path 'a\nb'", "path \"a\nb\""},
		{"single quote terminates bare words", "path'foo'", `path "foo"`},
		{"double quote terminates bare words", `path"foo"`, `path "foo"`},

		// Precedence: not > and > or; chains flatten to n-ary nodes.
		{"and chain flattens", "any and none and any", "and(any, none, any)"},
		{"or chain flattens", "any or none or any", "or(any, none, any)"},
		{"and binds tighter than or", "any and none or any and none", "or(and(any, none), and(any, none))"},
		{"not binds tighter than and", "not any and none", "and(not(any), none)"},
		{"not binds tighter than or", "not any or none", "or(not(any), none)"},
		{"double not", "not not any", "not(not(any))"},
		{"parens override", "(any or none) and any", "and(or(any, none), any)"},
		{"nested parens", "not (any and (none or any))", "not(and(any, or(none, any)))"},
		{"mixed symbols", "any && none || ! any", "or(and(any, none), not(any))"},
		{"regex in combinators", `not path regex "a|b" and any`, `and(not(path regex "a|b"), any)`},
		{"regex both fields in combinators", `path regex "a" and not agent regex "b" or path regex "c"`, `or(and(path regex "a", not(agent regex "b")), path regex "c")`},

		// Newlines are whitespace; positions reset per line (see errors).
		{"newlines separate tokens", "any and\n none", "and(any, none)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(tt.src)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.src, err)
			}
			if got := treeString(f); got != tt.want {
				t.Errorf("Parse(%q) tree = %s, want %s", tt.src, got, tt.want)
			}
		})
	}
}

func TestParseErrorPositions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // full Error() output: filter: L:C: message
	}{
		// Empty input errors point at end of input.
		{"empty expression", "", "filter: 1:1: empty filter expression"},
		{"whitespace expression", "  ", "filter: 1:3: empty filter expression"},

		// Lexer errors at the offending sequence.
		{"lone ampersand", "& any", `filter: 1:1: expected "&&"`},
		{"lone pipe", "| any", `filter: 1:1: expected "||"`},
		{"lone gt", "sent > 1", `filter: 1:6: expected ">="`},
		{"lone lt", "sent < 1", `filter: 1:6: expected "<="`},
		{"number out of range", "sent >= 9223372036854775808", `filter: 1:9: number "9223372036854775808" is out of range`},
		{"unterminated string", `path "abc`, "filter: 1:6: unterminated string"},
		{"unterminated single-quoted string", `path 'abc`, "filter: 1:6: unterminated string"},
		{"unterminated single-quoted string after newline", "any and\npath 'foo", "filter: 2:6: unterminated string"},
		{"trailing backslash in string", `path "abc\`, "filter: 1:6: unterminated string"},
		// "unknown escape sequence" is exclusive to the double-quoted spelling:
		// the same text in a single-quoted string is literal (see parse trees).
		{"bad escape points at escape char", `path "a\.b"`, `filter: 1:9: unknown escape sequence \.`},

		// Parser errors at the offending token.
		{"missing operand", "any and", "filter: 1:8: unexpected end of filter, expected an operand"},
		{"trailing token", "any any", `filter: 1:5: unexpected "any", expected end of filter`},
		{"unknown keyword", "foo", `filter: 1:1: unknown keyword "foo"`},
		{"not without operand", "not", "filter: 1:4: unexpected end of filter, expected an operand"},
		{"empty parens", "()", `filter: 1:2: unexpected ")", expected an operand`},
		{"unclosed paren points at opening paren", "(any", `filter: 1:5: missing ")" for "(" at 1:1`},

		// Primitive argument errors.
		{"src missing value", "src", `filter: 1:4: expected a bare IP address or prefix after "src", got end of filter`},
		{"src bad address", "src 1.2.3.4.5", `filter: 1:5: invalid IP address or prefix "1.2.3.4.5"`},
		{"src bad prefix length", "src 10.0.0.0/33", `filter: 1:5: invalid IP address or prefix "10.0.0.0/33"`},
		{"method missing value", "method", `filter: 1:7: expected a bare method name after "method", got end of filter`},
		{"method number value", "method 5m", `filter: 1:8: expected a bare method name after "method", got "5m"`},
		// A bare word cannot contain a quote character (a quote starts a string):
		// a method name carrying one loads today and now fails loudly at load.
		{"method name containing a quote", "method don't", `filter: 1:11: unterminated string`},
		{"method quoted value is not a bare word", `method 'GET'`, `filter: 1:8: expected a bare method name after "method", got "GET"`},
		{"path missing value", "path", `filter: 1:5: expected a quoted text value after "path", got end of filter`},
		{"path bare word value", "path x", `filter: 1:6: expected a quoted text value after "path", got "x"`},
		{"path prefix missing value", `path prefix`, `filter: 1:12: expected a quoted text value after "path", got end of filter`},
		{"path empty value", `path ""`, "filter: 1:6: path must not be empty"},
		// Empty text is rejected under either spelling, identically.
		{"path prefix empty value", `path prefix ""`, "filter: 1:13: prefix must not be empty"},
		{"path prefix empty single-quoted value", `path prefix ''`, "filter: 1:13: prefix must not be empty"},
		{"path suffix empty value", `path suffix ""`, "filter: 1:13: suffix must not be empty"},
		{"path suffix empty single-quoted value", `path suffix ''`, "filter: 1:13: suffix must not be empty"},
		{"path keyword empty value", `path keyword ""`, "filter: 1:14: keyword must not be empty"},
		{"path regex missing value", `path regex`, `filter: 1:11: expected a quoted text value after "path", got end of filter`},
		{"path regex empty value", `path regex ""`, "filter: 1:12: pattern must not be empty"},
		{"path regex invalid pattern", `path regex "("`, "filter: 1:12: error parsing regexp: missing closing ): `(`"},
		{"agent empty value", `agent ""`, "filter: 1:7: agent must not be empty"},
		{"agent prefix missing value", `agent prefix`, `filter: 1:13: expected a quoted text value after "agent", got end of filter`},
		{"agent prefix empty value", `agent prefix ""`, "filter: 1:14: prefix must not be empty"},
		{"agent prefix empty single-quoted value", `agent prefix ''`, "filter: 1:14: prefix must not be empty"},
		{"agent suffix missing value", `agent suffix`, `filter: 1:13: expected a quoted text value after "agent", got end of filter`},
		{"agent suffix empty value", `agent suffix ""`, "filter: 1:14: suffix must not be empty"},
		{"agent suffix empty single-quoted value", `agent suffix ''`, "filter: 1:14: suffix must not be empty"},
		{"agent regex missing value", `agent regex`, `filter: 1:12: expected a quoted text value after "agent", got end of filter`},
		{"agent regex empty value", `agent regex ""`, "filter: 1:13: pattern must not be empty"},
		{"agent regex invalid pattern", `agent regex "["`, "filter: 1:13: error parsing regexp: missing closing ]: `[`"},
		{"status missing value", "status", `filter: 1:7: expected a status code after "status", got end of filter`},
		{"status out of range", "status 20", "filter: 1:8: status must be within [100, 599]"},
		{"status size unit", "status 200k", `filter: 1:8: size units are only allowed after "sent" or "size"`},
		{"status class missing value", "status class", `filter: 1:13: expected a status class after "status", got end of filter`},
		{"status class out of range", "status class 9", "filter: 1:14: class must be within [1, 5]"},
		{"sent missing operator", "sent", `filter: 1:5: expected ">=" or "<=" after "sent", got end of filter`},
		{"sent bare number operator", "sent 5", `filter: 1:6: expected ">=" or "<=" after "sent", got "5"`},
		{"sent non-size value", `sent >= x`, `filter: 1:9: expected a size after "sent", got "x"`},
		{"sent decimal size is a word", "sent >= 0.1", `filter: 1:9: expected a size after "sent", got "0.1"`},
		{"sent MB spelling is a word", "sent >= 10MB", `filter: 1:9: expected a size after "sent", got "10MB"`},
		{"sent size overflow", "sent >= 9223372036854775807g", "filter: 1:9: size is out of range"},
		{"size missing operator", "size", `filter: 1:5: expected ">=", "<=", or "unknown" after "size", got end of filter`},
		{"size bare number operator", "size 5", `filter: 1:6: expected ">=", "<=", or "unknown" after "size", got "5"`},
		{"size known slip names unknown", "size known", `filter: 1:6: expected ">=", "<=", or "unknown" after "size", got "known"`},
		{"size bare word slip names unknown", "size maybe", `filter: 1:6: expected ">=", "<=", or "unknown" after "size", got "maybe"`},
		{"size known slip is case-sensitive", "size Known", `filter: 1:6: expected ">=", "<=", or "unknown" after "size", got "Known"`},
		{"size equality operator does not exist", "size = 5", `filter: 1:6: expected ">=", "<=", or "unknown" after "size", got "="`},
		{"size double equality operator does not exist", "size == 5", `filter: 1:6: expected ">=", "<=", or "unknown" after "size", got "=="`},
		{"size strict gt operator does not exist", "size > 5", `filter: 1:6: expected ">="`},
		{"size non-size value", `size >= x`, `filter: 1:9: expected a size after "size", got "x"`},
		{"size missing limit", `size >=`, `filter: 1:8: expected a size after "size", got end of filter`},
		{"size overflow", "size >= 9223372036854775807g", "filter: 1:9: size is out of range"},

		// Positions are 1-based rune counts; newlines reset the column.
		{"newline resets column", "any and\nfoo", `filter: 2:1: unknown keyword "foo"`},
		{"columns count runes", "path keyword \"\u00e4\" and foo", `filter: 1:22: unknown keyword "foo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.src)
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", tt.src)
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("Parse(%q) error %v is not a *ParseError", tt.src, err)
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("Parse(%q) error = %q, want %q", tt.src, got, tt.want)
			}
			if pe.Line < 1 || pe.Col < 1 {
				t.Errorf("ParseError position %d:%d is not 1-based", pe.Line, pe.Col)
			}
		})
	}
}

// Validation errors wrap their cause and stay unwrappable.
func TestParseErrorUnwrapsValidation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // substring of the wrapped validation error
	}{
		{"status out of range", "status 20", "status"},
		{"invalid regex pattern", `path regex "("`, "regexp"},
		{"empty single-quoted value", `path prefix ''`, "must not be empty"},
		{"size overflow", "size >= 9223372036854775807g", "out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.src)
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error %v is not a *ParseError", err)
			}
			if pe.Unwrap() == nil {
				t.Fatal("Unwrap() = nil, want the wrapped validation error")
			}
			if !strings.Contains(pe.Unwrap().Error(), tt.want) {
				t.Errorf("Unwrap() = %v, want it to contain %q", pe.Unwrap(), tt.want)
			}
		})
	}
}

// Example expressions parse.
func TestParseExamples(t *testing.T) {
	for _, src := range []string{
		`src 10.0.0.0/8 and (method GET or method HEAD) and path prefix "/api"`,
		`not agent keyword "curl" and sent >= 10m`,
		`size >= 100m and size <= 1g and path prefix "/releases/"`,
		`not size unknown and (size >= 1g or path prefix "/releases/")`,
		`agent prefix "curl/" or agent suffix ".0"`,
		`path regex "\\.(php|asp)$" or agent regex "(?i)bot"`,
		`path regex '\.(php|asp)$' or agent regex '(?i)bot'`,
		"status class 5 or none",
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
}
