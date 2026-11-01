# 03: docs/splint.md fixes

**What to build:** `docs/splint.md` only. (a) Scope the :52-53 claim
"skips individual malformed Reports with a log and a counter instead of
failing the cycle" to *semantically* invalid records (missing/unknown
penalty, bad rate, unparseable or fully-excluded target); state that a
structurally broken document (JSON type mismatch) fails the whole cycle
- deliberately, so corruption is loud. (b) Complete the merge-order
summary at :136-137: after prefix length descending and strictness,
equal-length groups order by rate ascending, then family. (c) :476
"the banned client's requests" -> "the banned Target's requests"
(Target is a prefix, not a host).

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent
