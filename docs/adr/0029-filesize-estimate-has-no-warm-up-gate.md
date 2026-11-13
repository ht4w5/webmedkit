# The Filesize estimate has no warm-up gate

The enrichment's warm-up gate - withholding a path's size until several
observations had averaged out the noise of transferred-bytes readings -
predated the move to response sizes (ADR 0019). With observations now
the exact sizes the server reports, the gate only delays knowledge: a
path's first downloads go unaccounted while replay stays blind to its
unknown size. The gate is gone: a path's estimate exists from its first
observation (seeded exactly, no blending) and each later observation
refines it by EWMA. Unknown now always means "this path has never been
observed", and nothing else.

Considered: a first-encounter-only bypass - enrich on the path's first
encounter, keep the gate for the encounters after it. Rejected: the
estimate would flicker known -> unknown -> known across encounters 1,
2-4, 5+, and empty content is indistinguishable from not-yet-known
(ADR 0024), so the flicker would be invisible in reports while replay
volumes oscillated. Waiting for more samples no longer buys accuracy
either: response sizes are exact as served.

Consequences: a path first observed under gzip seeds from its
compressed Content-Length and may undershoot until identity responses
blend in - accepted, with the increment cap at 1 absorbing the residual
skew (ADR 0019). Rollout ordering is unchanged: an un-upgraded emitter
leaves the estimate unknown, but one observation after the upgrade now
suffices instead of several.

Superseded by ADR 0032: the question is moot - there is no estimate to
warm up. Each response's served size is known at parse time, so a
path's first download counts at full weight immediately.
