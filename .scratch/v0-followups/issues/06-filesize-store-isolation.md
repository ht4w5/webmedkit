# 06: Filesize records isolated from the tracker store's LRU budget

**What to build:** A burst of distinct paths feeding the Filesize
estimate can no longer evict live tracker state. Today file-size
records live in the same sharded memory store as tracker records
(`memory.Filesize` adapts the same `*Store`): same entry cap, same
per-shard LRU eviction, no TTL. A flood of first-seen paths pushes
tracker records out of the shared budget and silently restarts
accumulations. The Filesize namespace gets its own store (its own
entry cap and eviction), so the two state kinds cannot compete; the
operator-facing cap knobs and Stats/metrics for the new budget follow
whatever pattern the tracker cache already sets.

Filed as follow-up from the enrichment-in-evaluation design session
(deliberately decoupled from that work; demand-driven enrichment
reduces entry churn but does not fix the shared-budget interference).

**Blocked by:** None (can start immediately)

**Status:** needs-triage

- [ ] Filesize records no longer share the tracker store's entry cap / LRU shards
- [ ] A distinct-path burst evicts no tracker records (race-enabled test proving the isolation)
- [ ] The Filesize budget is observable (entry cap and size on the Stats surface and metrics, mirroring the tracker cache block)
- [ ] Docs updated (storage/metrics docs as applicable)

## Comments
