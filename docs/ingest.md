# Ingest: choosing a transport

bandage receives events over syslog. `configs/bandage.yml` selects the
driver and transport; every syslog message must carry exactly one
access-log line (v0: nginx JSON in the bandage nginx log format, see
`ingest.parser`) - each message becomes one [Event](../GLOSSARY.md) in
the pipeline.

```yaml
ingest:
  driver: syslog               # driver-keyed; more drivers later
  syslog:
    network: unixgram          # tcp | udp | unixgram
    listen: /run/bandage/log.sock
  parser: nginx                # nginx JSON in the bandage nginx log format (flux-compatible)
```

`ingest.syslog.network` accepts `tcp`, `udp` or `unixgram`;
`ingest.syslog.listen` is the listen address (host:port, or a socket
path for unixgram). Unknown keys and invalid values abort startup.

## The bandage nginx log format

`ingest.parser: nginx` parses one JSON access-log line per message: the
bandage nginx log format. Bandage ships it ready to include -
[`configs/nginx/bandage_log.conf`](../configs/nginx/bandage_log.conf)
defines `log_format bandage escape=json` with exactly the parser's nine
mapped fields (`time`, `client`, `method`, `url`, `status`, `sent`,
`clen`, `crange`, `agent`), so wiring ingest is `include .../bandage_log.conf;` and
`access_log <path> bandage;` - no field contract to reverse-engineer.

`sent` is the response payload bytes the request actually transferred;
the two response-size fields carry the Filesize
([GLOSSARY.md](../GLOSSARY.md)) - the size as served with the
response - instead:

- `clen` (`$sent_http_content_length`) - the response's
  `Content-Length` header as a quoted string, `""` when the header was
  not sent. It is strict: a present value must be a non-negative
  integer - a negative or non-numeric `clen` is an invalid field and
  drops the line, counted like any other bad log line. Absent, empty or
  zero carries no size information.
- `crange` (`$sent_http_content_range`) - the response's
  `Content-Range` header as a quoted string, `""` when the header was
  not sent. It is lenient by contract: `bytes <a>-<b>/<total>` and
  `bytes */<total>` yield the file's total size; a `*` total and
  anything unparsable carry no size information and never drop the
  line.

The Filesize is derived from these two fields as served (GLOSSARY:
Filesize): a `206` takes its `crange` total only - the partial `clen`
would masquerade as a file size - and any other response its `clen`
when greater than zero; no method gating. The validation asymmetry is
deliberate: `clen` is a typed field, so a malformed value is an
invalid field and drops the line like any other; `crange` is free-form
header text, so an unusual value degrades to no size and never drops
the line. Sizes as served, never the bytes a request happened to
transfer.

Migration (flux-compatible): lines from an existing flux `log_format`
keep parsing unchanged - its extra `host`/`server`/`duration` fields
(and any others) are accepted and ignored, and lines without `clen` and
`crange` read as unknown sizes - while the `bandage` format
name never collides with an existing `flux` definition.

**Rollout ordering:** either order is safe - the `bandage` log format
keeps `clen` and `crange` unchanged, so upgrading the daemon alone
flips the semantics in one step and nothing on the emitter side.
Legacy flux lines read as unknown sizes (replay stays blind on that
traffic) until the emitter is upgraded; every upgraded line carries
its served size at once - no re-observation needed.

## Recommendation: udp or unixgram at high rate

For high-rate deployments run udp (remote senders) or unixgram (senders
on the same host). Both are datagram transports: **one datagram is one
message**. There is no per-sender connection state and no framing work
- the driver reads whole datagrams and forwards each message as it
arrives. unixgram skips the network stack entirely and is the cheapest
option when the log source is local; it is what
`configs/bandage.yml` ships with.

## The tcp cost

`network: tcp` is a byte stream, not messages: every message must be
framed out of the stream. The driver keeps one goroutine and one
scanner per connection and splits it into messages (newline framing,
RFC 6587 octet counting when detected), so:

- each sender costs a connection and a goroutine, and every message
  costs a framing scan where a datagram costs nothing;
- a stream is ordered and reliable: a stalled sender head-of-line
  blocks its own messages behind the ones it has not finished sending,
  and tcp's flow control turns an overloaded receiver into slow
  senders rather than visible loss;
- framing failures are stream-level: a message longer than the
  scanner's 64 KiB limit ends that connection's stream, taking the
  remaining messages with it.

Use tcp when senders are remote and require tcp's delivery behavior
(retries, congestion control) or can only speak tcp - not for
high-rate local feeds.

## The UDP kernel-buffer caveat

On udp, datagrams queue in a finite kernel receive buffer until the
driver reads them (the driver requests 64 KiB; the kernel clamps the
size - `net.core.rmem_max` on Linux). If bandage's reader lags behind a
burst, **the kernel drops datagrams from the queue - silently**:

- dropped datagrams never reach the pipeline, so they appear in
  neither `received` nor `dropped` on `GET /api/stats` (`dropped` counts
  parse failures only). Kernel loss is invisible in bandage's counters;
- watch the host's UDP counters instead - `nstat -az UdpRcvbufErrors`
  or `netstat -su` (receive buffer errors) - and treat any growth as
  lost events;
- to reduce it: raise `net.core.rmem_max` (and `net.core.rmem_default`)
  so the requested buffer is granted, keep senders burst-shy, and
  prefer unixgram on-host - its socket buffer pushes back on the
  sender (send blocks or fails) instead of discarding messages.

Nothing here is configurable inside bandage beyond the transport keys
above: the buffer and kernel behavior are the operating system's.
