# 01: Demote tracker cache Eviction logging to debug

**What to build:** Eviction events - tracker cache entries dropped because
the cache entry cap is reached - log at debug level instead of warn, so
steady-state cache pressure no longer floods production logs. Eviction
remains observable when an operator raises the log level to diagnose cache
sizing. No rate limiting or sampling is added.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Eviction events are emitted at debug level, not warn.
- [x] With a normal (non-debug) log level, evicting the cache produces no eviction lines.
- [x] At debug level, eviction lines are present and carry the same event context as before.
- [x] The daemon e2e log assertions verify the debug-level output and the warn-level silence.

## Answer

Implemented on `ticket/wam-01-demote-eviction-log-level` (commit
`6be0166`), merged into `integration/web-panel-api-move` (`af35f2e`).
The eviction record drops `Warn()` to `Debug()` in
`bandage/internal/infra/cache/memory/memory.go`; the message and
`namespace`/`target`/`path` attrs are untouched, so the event context
is the same as before. The new `TestE2EEvictionLoggedAtDebugLevel`
asserts the record level is exactly debug (one line, same attrs), the
line is present when the level is raised to debug, and absent at warn
level. No rate limiting or sampling. Reviewed in the fix pass `62e3efa`.
