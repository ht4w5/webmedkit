// Package nginx parses nginx JSON access-log lines in the bandage nginx
// log format (flux-compatible) into domain events: required fields
// validated, reserved and unknown ignored.
package nginx

import (
	"bytes"
	jsonv1 "encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bytedance/sonic"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// lineDecoder decodes log lines with bytedance/sonic, configured to the
// historic map-decoder contract: member names match exactly (case-sensitive,
// never the case-insensitive default) and unescaped control characters still
// fail the line. Invalid UTF-8 in strings is mangled to U+FFFD (see
// fieldValue.UnmarshalJSON), never a dropped line.
var lineDecoder = sonic.Config{
	ValidateString: true,
	CaseSensitive:  true,
}.Froze()

// FieldError reports an invalid required (or typed) field; Field and Msg are
// the parser's field-level error contract (drop accounting counts all errors alike).
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("invalid field %q: %s", e.Field, e.Msg)
}

// Parser parses nginx JSON log lines into events. It is stateless and
// safe for concurrent use.
type Parser struct{}

// New returns a new nginx JSON parser.
func New() *Parser { return &Parser{} }

// fieldValue is one captured JSON member value: strings decode once into str,
// other kinds keep exact bytes in raw (zero-copy for Parse's duration).
type fieldValue struct {
	str     string
	raw     []byte
	isStr   bool
	present bool
}

// UnmarshalJSON captures the member's value without copying (except the
// decoded string) or validating; last member wins on duplicate names.
func (v *fieldValue) UnmarshalJSON(b []byte) error {
	*v = fieldValue{present: true}
	if len(b) > 0 && b[0] == '"' {
		s, err := decodeString(b)
		if err != nil {
			return err
		}
		v.str = sanitizeUTF8(s)
		v.isStr = true
		return nil
	}
	v.raw = b
	return nil
}

// decodeString unescapes a JSON string value. Plain strings (no escapes)
// are copied directly; only escaped strings pay for a decoder pass.
func decodeString(b []byte) (string, error) {
	body := b[1 : len(b)-1]
	if bytes.IndexByte(body, '\\') < 0 {
		return string(body), nil
	}
	var s string
	if err := lineDecoder.Unmarshal(b, &s); err != nil {
		return "", err
	}
	return s, nil
}

// sanitizeUTF8 replaces each invalid UTF-8 byte with U+FFFD, keeping the
// historic decoder's string mangling (one replacement per invalid byte).
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + utf8.RuneLen(utf8.RuneError))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(string(utf8.RuneError))
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// isNull reports whether the member was present as a JSON null.
func isNull(v fieldValue) bool {
	return v.present && !v.isStr && string(v.raw) == "null"
}

// text returns the member's value as text: the decoded string, or the
// exact raw JSON bytes for other kinds.
func (v fieldValue) text() string {
	if v.isStr {
		return v.str
	}
	return string(v.raw)
}

// rawFields holds the captured values of the known log fields; unknown and
// reserved fields are never captured.
type rawFields struct {
	Time   fieldValue `json:"time"`
	Client fieldValue `json:"client"`
	Method fieldValue `json:"method"`
	URL    fieldValue `json:"url"`
	Status fieldValue `json:"status"`
	Sent   fieldValue `json:"sent"`
	Clen   fieldValue `json:"clen"`
	Crange fieldValue `json:"crange"`
	Agent  fieldValue `json:"agent"`
}

// Parse maps one raw log line onto a domain Event, validating required fields;
// malformed lines and invalid fields return an error and the event is dropped.
func (p *Parser) Parse(line []byte) (domain.Event, error) {
	var fields rawFields
	if err := lineDecoder.Unmarshal(line, &fields); err != nil {
		return domain.Event{}, fmt.Errorf("parse nginx json: %w", legacyLineError(line, err))
	}

	ev := domain.Event{}

	raw, err := require(fields.Time, "time")
	if err != nil {
		return domain.Event{}, err
	}
	if ev.Time, err = parseTime(raw); err != nil {
		return domain.Event{}, &FieldError{Field: "time", Msg: err.Error()}
	}

	if raw, err = require(fields.Client, "client"); err != nil {
		return domain.Event{}, err
	}
	s, err := parseString(raw)
	if err != nil {
		return domain.Event{}, &FieldError{Field: "client", Msg: err.Error()}
	}
	if ev.RemoteAddr, err = netip.ParseAddr(s); err != nil {
		return domain.Event{}, &FieldError{Field: "client", Msg: "not an IP address"}
	}

	if raw, err = require(fields.Method, "method"); err != nil {
		return domain.Event{}, err
	}
	if ev.Method, err = parseNonEmptyString(raw); err != nil {
		return domain.Event{}, &FieldError{Field: "method", Msg: err.Error()}
	}

	if raw, err = require(fields.URL, "url"); err != nil {
		return domain.Event{}, err
	}
	if ev.Path, err = parseNonEmptyString(raw); err != nil {
		return domain.Event{}, &FieldError{Field: "url", Msg: err.Error()}
	}

	if raw, err = require(fields.Status, "status"); err != nil {
		return domain.Event{}, err
	}
	n, err := parseInt(raw)
	if err != nil {
		return domain.Event{}, &FieldError{Field: "status", Msg: err.Error()}
	}
	if n < 100 || n > 599 {
		return domain.Event{}, &FieldError{Field: "status", Msg: "out of range 100-599"}
	}
	ev.Status = int(n)

	if raw, err = require(fields.Sent, "sent"); err != nil {
		return domain.Event{}, err
	}
	sent, err := parseInt(raw)
	if err != nil {
		return domain.Event{}, &FieldError{Field: "sent", Msg: err.Error()}
	}
	if sent < 0 {
		return domain.Event{}, &FieldError{Field: "sent", Msg: "must be non-negative"}
	}
	ev.SentBytes = sent

	// clen is optional (absent, null, "" = no size information); when present
	// it must be a non-negative integer, strict like sent - malformed drops the
	// line. It is parse-time state feeding the Filesize derivation below, not
	// Event state.
	var contentLength int64
	if raw = fields.Clen; raw.present && !isNull(raw) {
		if contentLength, err = parseContentLength(raw); err != nil {
			return domain.Event{}, &FieldError{Field: "clen", Msg: err.Error()}
		}
	}

	// crange is optional and lenient: "bytes <a>-<b>/<total>" and "bytes
	// */<total>" yield the total; anything else yields 0 and never drops the
	// line. Like clen, it only feeds the derivation below.
	var contentRangeTotal int64
	if raw = fields.Crange; raw.present && !isNull(raw) {
		contentRangeTotal = parseContentRangeTotal(raw)
	}

	// The Filesize is the size as served with this response (GLOSSARY:
	// Filesize): a 206 takes its Content-Range total only - its partial
	// Content-Length would masquerade as a file size - any other
	// response its Content-Length when greater than zero. No method or
	// status gating beyond the 206 rule; an unusable or absent source
	// stays unknown (zero means unknown, ADR 0024).
	switch {
	case ev.Status == 206 && contentRangeTotal > 0:
		ev.SizeBytes = contentRangeTotal
	case ev.Status != 206 && contentLength > 0:
		ev.SizeBytes = contentLength
	}

	// agent is optional; when present it must be a string (null = absent).
	if raw = fields.Agent; raw.present && !isNull(raw) {
		if ev.Agent, err = parseString(raw); err != nil {
			return domain.Event{}, &FieldError{Field: "agent", Msg: err.Error()}
		}
	}

	// host, server, duration are accepted and ignored; unknown fields too.
	return ev, nil
}

// legacyLineError re-renders a decode failure as the historic decoder's
// encoding/json v1 text, keeping the drop-log message byte-identical: the
// line is replayed through the v1 decoder and its error taken as the text.
// Sonic's own error wording is only a fallback when the two disagree.
func legacyLineError(line []byte, err error) error {
	var raw map[string]jsonv1.RawMessage
	if verr := jsonv1.Unmarshal(line, &raw); verr != nil {
		return verr
	}
	return err
}

// require returns a required field's value; absent and null fields report as missing.
func require(v fieldValue, name string) (fieldValue, error) {
	if !v.present || isNull(v) {
		return fieldValue{}, &FieldError{Field: name, Msg: "required field missing"}
	}
	return v, nil
}

func parseString(v fieldValue) (string, error) {
	if !v.isStr {
		return "", errors.New("must be a string")
	}
	return v.str, nil
}

func parseNonEmptyString(v fieldValue) (string, error) {
	s, err := parseString(v)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", errors.New("must not be empty")
	}
	return s, nil
}

func parseInt(v fieldValue) (int64, error) {
	// A JSON string never parses as an integer, whatever its content.
	if v.isStr {
		return 0, errors.New("must be an integer")
	}
	n, err := strconv.ParseInt(string(v.raw), 10, 64)
	if err != nil {
		return 0, errors.New("must be an integer")
	}
	return n, nil
}

// parseContentLength parses the clen field: a quoted integer string ("" =
// header not sent) or bare number; only non-negative integers are valid.
func parseContentLength(v fieldValue) (int64, error) {
	text := v.text()
	if text == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, errors.New("must be an integer")
	}
	if n < 0 {
		return 0, errors.New("must be non-negative")
	}
	return n, nil
}

// parseContentRangeTotal extracts the file's total from the crange field's
// Content-Range value; anything unparsable yields 0 and never drops the line.
func parseContentRangeTotal(v fieldValue) int64 {
	text := v.text()
	const unit = "bytes "
	if len(text) < len(unit) || !strings.EqualFold(text[:len(unit)], unit) {
		return 0
	}
	span, total, ok := strings.Cut(text[len(unit):], "/")
	if !ok {
		return 0
	}
	if span != "*" {
		lo, hi, ok := strings.Cut(span, "-")
		if !ok {
			return 0
		}
		if _, ok := parseSizeDigits(lo); !ok {
			return 0
		}
		if _, ok := parseSizeDigits(hi); !ok {
			return 0
		}
	}
	n, ok := parseSizeDigits(total)
	if !ok {
		return 0
	}
	return n
}

// parseSizeDigits parses a non-negative decimal size (up to int64); empty,
// signed, fractional and overflowing values fail.
func parseSizeDigits(s string) (int64, bool) {
	n, err := strconv.ParseUint(s, 10, 63)
	if err != nil {
		return 0, false
	}
	return int64(n), true
}

// parseTime parses the $msec time: Unix seconds with an optional
// fraction, as a JSON string or JSON number. The fraction is kept.
func parseTime(v fieldValue) (time.Time, error) {
	text := v.text()
	if text == "" || strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return time.Time{}, errors.New("must be Unix seconds with fraction")
	}

	secPart := text
	var nsec int64
	if i := strings.IndexByte(text, '.'); i >= 0 {
		secPart = text[:i]
		frac := text[i+1:]
		if frac == "" {
			return time.Time{}, errors.New("empty fraction")
		}
		for _, r := range frac {
			if !unicode.IsDigit(r) {
				return time.Time{}, errors.New("bad fraction")
			}
		}
		var buf [9]byte // nanoseconds: fractions are truncated or padded to 9 digits
		w := copy(buf[:], frac)
		for ; w < len(buf); w++ {
			buf[w] = '0'
		}
		n, err := strconv.ParseInt(string(buf[:]), 10, 64)
		if err != nil {
			return time.Time{}, errors.New("bad fraction")
		}
		nsec = n
	}

	sec, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil || secPart == "" {
		return time.Time{}, errors.New("must be Unix seconds with fraction")
	}
	return time.Unix(sec, nsec).UTC(), nil
}
