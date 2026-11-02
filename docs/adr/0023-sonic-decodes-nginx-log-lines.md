# Sonic decodes nginx log lines

The nginx log-parse path decodes line JSON with bytedance/sonic (JIT,
amd64 is the deployment target) instead of the standard library decoder -
the single decode call in `bandage/internal/infra/parse/nginx` and
nothing else in the codebase. The swap is behavior-preserving: the
historic map-decoder contract survives in the decode configuration
(`CaseSensitive`, `ValidateString`), in `fieldValue`'s capture semantics
(duplicate names resolve last-wins, strings mangle each invalid UTF-8
byte to U+FFFD, non-string values keep their exact bytes), and in the
drop-log error text, which stays byte-identical to the historic
encoding/json v1 wording because a decode failure replays the line
through the v1 decoder and takes its error as the text. Throughput over
the shared bench corpus rises 24% (minimal lines) to 36% (full lines)
lines/s - the whole justification for the dependency. The dependency is
vendored like the tree's existing third-party dependencies.

Considered: keeping encoding/json v2 + jsontext (rejected: the throughput
the ingest story asks for is not there - that decoder is the baseline
being beaten); rendering the v1 error text by mapping sonic's error types
(rejected: sonic reports generic "syntax error at index" codes, so
matching v1's per-case wording - "invalid character 'o' in literal null
(expecting 'u')" - would mean reimplementing v1's scanner messages by
hand, and any drift is invisible until a line drops in production; the v1
replay reproduces the text by construction, including v1's whole-line
syntax scan winning over type errors); sonic's `unquote` subpackage for
string unescape (rejected: it always allocates and its error convention
returns a non-nil "ok"; instead plain strings unescape with a direct
copy and only escaped strings pay for a decoder pass).

Consequences: every Parse allocates one extra copy of the line (sonic's
entry point converts []byte to string) and two more allocations per line
overall (+37-62% bytes/op on the corpus) - a real GC cost paid for
throughput. Sonic's defaults are not the historic contract: it matches
member names case-insensitively and accepts unescaped control characters,
so the frozen config pins `CaseSensitive` and `ValidateString`; dropping
either flag silently breaks behavior the unchanged parser tests guard.
The standard library decoder survives only on the drop path, replaying
failed lines for their error text; if sonic ever rejects a line the v1
decoder accepts, sonic's own error text is the fallback. Recorded
because both survivors - the v1 replay and the config flags - look
removable to a future reader who sees only "sonic replaced the stdlib
decoder".
