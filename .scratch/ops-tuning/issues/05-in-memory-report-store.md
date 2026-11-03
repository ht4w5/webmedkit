# 05: In-memory report store for debugging

**What to build:** A developer debugging bandage can point the daemon at
a throwaway report store with one config option in the sqlite driver's
settings: memory mode. In memory mode the report repository runs against
an in-memory database - one shared database across the write and read
paths, no files left behind, an empty store on every start. The config
rejects memory mode together with the store's path setting: setting both
is a validation error naming both keys, so nobody has to guess which one
won. Starting in memory mode logs a warning that reports are not durable
- this deliberately bends ADR 0002 for debugging, and the daemon says so
at the moment the operator can still act on it. The existing way of
naming an in-memory database via the path keeps working unchanged.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Memory mode alone in the config parses and the daemon serves
  reports against the in-memory store: upserts, listing, deletion, and
  expiry sweeping all work.
- [x] Memory mode together with a store path is a config validation
  error.
- [x] Memory mode with the wrong driver is rejected like the rest of the
  store config.
- [x] Startup in memory mode logs the non-durable-reports warning.
- [x] A file-backed run logs no such warning and behaves as before.
- [x] Config tests cover the parse and rejection cases; a daemon e2e
  test covers the memory-mode round trip and the startup warning; all
  existing tests pass.

## Comments

Resolved on `integration/ops-tuning`: merged in `15d573b` (merge: 05-in-memory-report-store into integration/ops-tuning (in-memory report store for debugging)); code-review fix pass merged in `1d261e2`. All checklist items are pinned by the tests named in the spec's Testing Decisions. Interactive panel behaviour remains on the manual user-story checklist per the panel's testing convention.
