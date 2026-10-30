package nginx

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fullLine is a full-shape bandage nginx log format line (optional, reserved
// and unknown fields included).
const fullLine = `{"time":"1734345934.123","client":"203.0.113.7","server":"203.0.113.1","method":"GET","url":"/releases/24.04.iso","duration":"0.125","host":"example.org","status":200,"sent":123456,"clen":"234567","crange":"bytes 0-234566/345678","agent":"Wget/1.21","extra":"ignored"}`

func TestParseFieldMapping(t *testing.T) {
	ev, err := New().Parse([]byte(fullLine))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if want := time.Unix(1734345934, 123000000).UTC(); !ev.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", ev.Time, want)
	}
	if got := ev.RemoteAddr.String(); got != "203.0.113.7" {
		t.Errorf("RemoteAddr = %q, want %q", got, "203.0.113.7")
	}
	if ev.Method != "GET" {
		t.Errorf("Method = %q, want %q", ev.Method, "GET")
	}
	if ev.Path != "/releases/24.04.iso" {
		t.Errorf("Path = %q, want %q", ev.Path, "/releases/24.04.iso")
	}
	if ev.Status != 200 {
		t.Errorf("Status = %d, want 200", ev.Status)
	}
	if ev.SentBytes != 123456 {
		t.Errorf("SentBytes = %d, want 123456", ev.SentBytes)
	}
	if ev.ContentLength != 234567 {
		t.Errorf("ContentLength = %d, want 234567", ev.ContentLength)
	}
	if ev.ContentRangeTotal != 345678 {
		t.Errorf("ContentRangeTotal = %d, want 345678", ev.ContentRangeTotal)
	}
	if ev.Agent != "Wget/1.21" {
		t.Errorf("Agent = %q, want %q", ev.Agent, "Wget/1.21")
	}
	if ev.SizeBytes != 0 {
		t.Errorf("SizeBytes = %d, want 0 (file size unknown until enrichment)", ev.SizeBytes)
	}
}

// shippedFormat is the log_format template the shipped nginx snippet
// (configs/nginx/bandage_log.conf) must define.
const shippedFormat = `{"time":"$msec","client":"$remote_addr","method":"$request_method","url":"$uri","status":$status,"sent":$body_bytes_sent,"clen":"$sent_http_content_length","crange":"$sent_http_content_range","agent":"$http_user_agent"}`

// shippedLogFormat returns the template from the shipped nginx snippet; the
// anchored match also pins the file to exactly one directive, no comments
// and no vhost. The extraction regex and the render-and-check flow in
// TestParseShippedLogFormatLine are intentionally duplicated in
// internal/app/shippedformat_test.go (shippedLogLine): the two sides sit in
// different test packages, and a shared helper would need an exported
// package that exists purely for tests. Keep both copies in sync with the
// shipped snippet's contract (configs/nginx/bandage_log.conf).
func shippedLogFormat(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../../../configs/nginx/bandage_log.conf")
	if err != nil {
		t.Fatalf("read shipped snippet: %v", err)
	}
	pat := regexp.MustCompile(`\Alog_format bandage escape=json '([^']*)';\z`)
	m := pat.FindStringSubmatch(strings.TrimRight(string(data), "\n"))
	if m == nil {
		t.Fatalf("shipped snippet = %q, want one log_format bandage escape=json '<format>'; directive", data)
	}
	return m[1]
}

// TestShippedLogFormatConf pins the shipped snippet to the parser's field
// contract: one log_format bandage escape=json directive, nothing else.
func TestShippedLogFormatConf(t *testing.T) {
	if got := shippedLogFormat(t); got != shippedFormat {
		t.Errorf("shipped log format = %q, want %q", got, shippedFormat)
	}
}

// TestParseShippedLogFormatLine renders one line the way nginx renders the
// shipped format and proves it parses into a mapped Event, agent included:
// agent is the field agent-based filters (the tripwire pattern) read.
func TestParseShippedLogFormatLine(t *testing.T) {
	line := strings.NewReplacer(
		"$msec", "1734345934.123",
		"$remote_addr", "203.0.113.7",
		"$request_method", "GET",
		"$uri", "/releases/24.04.iso",
		"$status", "200",
		"$body_bytes_sent", "123456",
		"$sent_http_content_length", "234567",
		"$sent_http_content_range", "bytes 0-234566/234567",
		"$http_user_agent", "curl/8.5.0",
	).Replace(shippedLogFormat(t))
	if strings.Contains(line, "$") {
		t.Fatalf("rendered line %q leaves a variable unsubstituted", line)
	}
	ev, err := New().Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if want := time.Unix(1734345934, 123000000).UTC(); !ev.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", ev.Time, want)
	}
	if got := ev.RemoteAddr.String(); got != "203.0.113.7" {
		t.Errorf("RemoteAddr = %q, want %q", got, "203.0.113.7")
	}
	if ev.Method != "GET" {
		t.Errorf("Method = %q, want %q", ev.Method, "GET")
	}
	if ev.Path != "/releases/24.04.iso" {
		t.Errorf("Path = %q, want %q", ev.Path, "/releases/24.04.iso")
	}
	if ev.Status != 200 {
		t.Errorf("Status = %d, want 200", ev.Status)
	}
	if ev.SentBytes != 123456 {
		t.Errorf("SentBytes = %d, want 123456", ev.SentBytes)
	}
	if ev.ContentLength != 234567 {
		t.Errorf("ContentLength = %d, want 234567", ev.ContentLength)
	}
	if ev.ContentRangeTotal != 234567 {
		t.Errorf("ContentRangeTotal = %d, want 234567 (the total the shipped crange carries)", ev.ContentRangeTotal)
	}
	if ev.Agent != "curl/8.5.0" {
		t.Errorf("Agent = %q, want %q", ev.Agent, "curl/8.5.0")
	}
}

// TestParseContentLengthField pins the clen mapping: quoted integer string
// ("" = header not sent); absent, empty and zero all mean "no size information".
func TestParseContentLengthField(t *testing.T) {
	tests := []struct {
		name string
		clen string // the raw JSON member; "<absent>" omits it
		want int64
	}{
		{name: "absent", clen: "<absent>", want: 0},
		{name: "null", clen: `null`, want: 0},
		{name: "empty string", clen: `""`, want: 0},
		{name: "zero", clen: `"0"`, want: 0},
		{name: "length", clen: `"4000"`, want: 4000},
		{name: "bare number", clen: `4000`, want: 4000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0`
			if tt.clen != "<absent>" {
				line += `,"clen":` + tt.clen
			}
			line += `}`
			ev, err := New().Parse([]byte(line))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if ev.ContentLength != tt.want {
				t.Errorf("ContentLength = %d, want %d", ev.ContentLength, tt.want)
			}
		})
	}
}

// TestParseContentRangeField pins the crange mapping: the extracted total.
// Every row below must parse - unusual values yield 0, never a dropped line.
func TestParseContentRangeField(t *testing.T) {
	tests := []struct {
		name   string
		crange string // the raw JSON member; "<absent>" omits it
		want   int64
	}{
		{name: "absent", crange: "<absent>", want: 0},
		{name: "null", crange: `null`, want: 0},
		{name: "empty string", crange: `""`, want: 0},
		{name: "range with total", crange: `"bytes 0-499/1234"`, want: 1234},
		{name: "middle range with total", crange: `"bytes 500-999/1234"`, want: 1234},
		{name: "unsatisfied range with total", crange: `"bytes */1234"`, want: 1234},
		{name: "unit case-insensitive", crange: `"Bytes 0-499/1234"`, want: 1234},
		{name: "star total", crange: `"bytes 0-499/*"`, want: 0},
		{name: "star range and star total", crange: `"bytes */*"`, want: 0},
		{name: "zero total", crange: `"bytes 0-499/0"`, want: 0},
		{name: "no total", crange: `"bytes 0-499"`, want: 0},
		{name: "empty total", crange: `"bytes 0-499/"`, want: 0},
		{name: "garbage range part", crange: `"bytes abc-def/1234"`, want: 0},
		{name: "signed range bound", crange: `"bytes -1-499/1234"`, want: 0},
		{name: "non-numeric total", crange: `"bytes 0-499/abc"`, want: 0},
		{name: "overflowing total", crange: `"bytes 0-499/99999999999999999999"`, want: 0},
		{name: "trailing total junk", crange: `"bytes 0-499/1234x"`, want: 0},
		{name: "unknown unit", crange: `"items 0-499/1234"`, want: 0},
		{name: "missing unit", crange: `"0-499/1234"`, want: 0},
		{name: "bare number", crange: `1234`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":206,"sent":0`
			if tt.crange != "<absent>" {
				line += `,"crange":` + tt.crange
			}
			line += `}`
			ev, err := New().Parse([]byte(line))
			if err != nil {
				t.Fatalf("Parse error (unusual Content-Range values must never drop the line): %v", err)
			}
			if ev.ContentRangeTotal != tt.want {
				t.Errorf("ContentRangeTotal = %d, want %d", ev.ContentRangeTotal, tt.want)
			}
		})
	}
}

// TestParseOldFluxLineUnchanged: a line in the old flux log format (no
// clen or crange) keeps parsing unchanged and carries no size information.
func TestParseOldFluxLineUnchanged(t *testing.T) {
	line := `{"time":"1734345934.123","client":"203.0.113.7","method":"GET","url":"/releases/24.04.iso","status":200,"sent":123456,"agent":"Wget/1.21"}`
	ev, err := New().Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if ev.ContentLength != 0 {
		t.Errorf("ContentLength = %d, want 0 (old flux lines carry no response length)", ev.ContentLength)
	}
	if ev.ContentRangeTotal != 0 {
		t.Errorf("ContentRangeTotal = %d, want 0 (old flux lines carry no response range)", ev.ContentRangeTotal)
	}
	if ev.SentBytes != 123456 || ev.Path != "/releases/24.04.iso" || ev.Agent != "Wget/1.21" {
		t.Errorf("event = %+v, want the old line's fields unchanged", ev)
	}
}

func TestParseAgentOptional(t *testing.T) {
	line := `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0}`
	ev, err := New().Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if ev.Agent != "" {
		t.Errorf("Agent = %q, want empty", ev.Agent)
	}
	if want := time.Unix(1734345934, 0).UTC(); !ev.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", ev.Time, want)
	}
}

func TestParseTimeAcceptsJSONNumber(t *testing.T) {
	line := `{"time":1734345934.5,"client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0}`
	ev, err := New().Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if want := time.Unix(1734345934, 500000000).UTC(); !ev.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", ev.Time, want)
	}
}

func TestParseIPv6Client(t *testing.T) {
	line := `{"time":"1734345934","client":"2001:db8::7","method":"GET","url":"/","status":200,"sent":0}`
	ev, err := New().Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got := ev.RemoteAddr.String(); got != "2001:db8::7" {
		t.Errorf("RemoteAddr = %q, want %q", got, "2001:db8::7")
	}
}

func TestParseMalformedLine(t *testing.T) {
	for _, line := range []string{
		``,
		`not json`,
		`{"time":`,
		`[]`,
		`5`,
	} {
		_, err := New().Parse([]byte(line))
		if err == nil {
			t.Errorf("Parse(%q) succeeded, want error", line)
			continue
		}
		var fe *FieldError
		if errors.As(err, &fe) {
			t.Errorf("Parse(%q) error %v is a *FieldError, want malformed-line error", line, err)
		}
	}
}

// TestParseMalformedLineErrorText pins the malformed-line error text to the
// historic decoder's (encoding/json v1 wording), byte for byte.
func TestParseMalformedLineErrorText(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		// Byte-identical with the historic decoder (encoding/json v1 text).
		{"empty", ``, `parse nginx json: unexpected end of JSON input`},
		{"truncated object", `{"time":`, `parse nginx json: unexpected end of JSON input`},
		{"truncated at open", `{`, `parse nginx json: unexpected end of JSON input`},
		{"truncated literal", `tru`, `parse nginx json: unexpected end of JSON input`},
		{"truncated string", `"unterminated`, `parse nginx json: unexpected end of JSON input`},
		{"truncated value", `{"time":1`, `parse nginx json: unexpected end of JSON input`},
		{"literal typo keeps its expectation", `not json`, `parse nginx json: invalid character 'o' in literal null (expecting 'u')`},
		{"missing value", `{"time":}`, `parse nginx json: invalid character '}' looking for beginning of value`},
		{"garbage prefix", `xyzzy}`, `parse nginx json: invalid character 'x' looking for beginning of value`},
		{"missing colon", `{"time" 1}`, `parse nginx json: invalid character '1' after object key`},
		{"missing comma", `{"time":"1" "client":"x"}`, `parse nginx json: invalid character '"' after object key:value pair`},
		{"trailing garbage", `{"time":"1"} 0`, `parse nginx json: invalid character '0' after top-level value`},

		// Type errors keep the old decode target's name and v1's kind
		// words; a syntax error anywhere still wins (v1 scanned the whole line first).
		{"top-level array", `[]`, `parse nginx json: json: cannot unmarshal array into Go value of type map[string]jsontext.Value`},
		{"top-level number", `5`, `parse nginx json: json: cannot unmarshal number into Go value of type map[string]jsontext.Value`},
		{"top-level bool", `true`, `parse nginx json: json: cannot unmarshal bool into Go value of type map[string]jsontext.Value`},
		{"top-level string", `""`, `parse nginx json: json: cannot unmarshal string into Go value of type map[string]jsontext.Value`},
		{"number with trailing garbage", `00`, `parse nginx json: invalid character '0' after top-level value`},
		{"number with trailing junk", `5x`, `parse nginx json: invalid character 'x' after top-level value`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New().Parse([]byte(tt.line))
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", tt.line)
			}
			if err.Error() != tt.want {
				t.Errorf("Parse(%q) error = %q, want %q", tt.line, err.Error(), tt.want)
			}
		})
	}
}

func TestParseInvalidRequiredField(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		field string
	}{
		{name: "missing time", field: "time", line: `{"client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0}`},
		{name: "missing client", field: "client", line: `{"time":"1734345934","method":"GET","url":"/","status":200,"sent":0}`},
		{name: "missing method", field: "method", line: `{"time":"1734345934","client":"203.0.113.7","url":"/","status":200,"sent":0}`},
		{name: "missing url", field: "url", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","status":200,"sent":0}`},
		{name: "missing status", field: "status", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","sent":0}`},
		{name: "missing sent", field: "sent", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200}`},
		{name: "bad time", field: "time", line: `{"time":"abc","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0}`},
		{name: "bad time fraction", field: "time", line: `{"time":"1734345934.1x","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0}`},
		{name: "time wrong type", field: "time", line: `{"time":{"sec":1},"client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0}`},
		{name: "bad client", field: "client", line: `{"time":"1734345934","client":"not-an-ip","method":"GET","url":"/","status":200,"sent":0}`},
		{name: "empty method", field: "method", line: `{"time":"1734345934","client":"203.0.113.7","method":"","url":"/","status":200,"sent":0}`},
		{name: "empty url", field: "url", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"","status":200,"sent":0}`},
		{name: "status below range", field: "status", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":99,"sent":0}`},
		{name: "status above range", field: "status", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":600,"sent":0}`},
		{name: "status as string", field: "status", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":"200","sent":0}`},
		{name: "sent negative", field: "sent", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":-1}`},
		{name: "sent fractional", field: "sent", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":1.5}`},
		{name: "agent wrong type", field: "agent", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0,"agent":7}`},
		{name: "clen negative", field: "clen", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0,"clen":"-1"}`},
		{name: "clen non-numeric", field: "clen", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0,"clen":"abc"}`},
		{name: "clen fractional", field: "clen", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0,"clen":"1.5"}`},
		{name: "clen wrong type", field: "clen", line: `{"time":"1734345934","client":"203.0.113.7","method":"GET","url":"/","status":200,"sent":0,"clen":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New().Parse([]byte(tt.line))
			if err == nil {
				t.Fatalf("Parse succeeded, want error on field %q", tt.field)
			}
			var fe *FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("Parse error %v is not a *FieldError", err)
			}
			if fe.Field != tt.field {
				t.Errorf("FieldError.Field = %q, want %q", fe.Field, tt.field)
			}
		})
	}
}
