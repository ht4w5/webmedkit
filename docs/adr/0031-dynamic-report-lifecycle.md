# Dynamic report lifecycle: value-bound death, grace margin, decay-driven Downgrade

A report action's `ttl` is a wall-clock lifetime, and for penalties
that track traffic behavior that is the wrong shape: when a bandwidth
tracker overflows a volume and the accumulation then leaks back below
it - the client stopped, the abuse is over - the Report stays enforced
until the clock runs out. Enforcement outlives its justification. A
report action therefore declares one of two lifecycle policies (Model
M): `ttl` written is exactly today's fixed semantics - immune to
decay, dies at `updated_at + ttl`, byte-identical for every existing
rules file - and `ttl` absent writes a **Dynamic report**, alive while
its Tracker value is strictly above its death line and dead at
**Underflow**, when the value decays to it. The policy always belongs
to the report action that last wrote or restated the stance (Trigger
override and Downgrade alike), and the boundary is strict - alive only
while the value is strictly above the line - mirroring Overflow's
strictness. Lifecycle acts are materialized lazily, at document
generation and at sweep time, against live tracker state: nothing runs
between events and no timer exists.

**Grace versus margin.** An optional `grace` widens the death line
from the recorded volume to `volume - leak x grace`, so a brief pause
in otherwise sustained abuse does not withdraw and re-flag enforcement
in a flap. Under linear decay the two are one quantity - dying "grace
after underflow" is dying at `volume - leak x grace` - so `grace`
compiles to a margin at rule build and no lifecycle state exists. The
one behavioral difference from a "true" grace window is the accepted
trickle-hover band: traffic that parks the value just below the volume
keeps the Report alive inside the margin, where a true grace would
kill it. An edge on record: a margin wider than the volume leaves a
negative death line, and such a Report never dies by decay at all -
deletion and the startup sweep still end it. Load does not refuse it;
accepted.

A ladder's dynamic stance **Downgrades** as the value decays: when the
value crosses the stance's death line and a lower rung is still
exceeded, the Report restates itself to the highest rung still
exceeded - reason and Penalty move with a fresh `updated_at`, the
recorded volume restates to that rung's threshold, `created_at` and
the Sample stay (the climb event crossed every lower rung) - and the
stance adopts the target rung's policy: a fixed rung's `ttl` runs from
the downgrade write, a dynamic rung keeps value-bound liveness. The
margin guards the downgrade line too, so stances do not flap between
rungs at the boundary; the Report dies, strict, when no rung remains
exceeded. This amends the escalation-ladder decision (ADR 0013): its
suppression clause narrows to no downgrade *from Triggers* -
decay-driven Downgrade is the inverse, written lazily against live
tracker state and never by a Trigger. Fixed stances never
decay-interact: no Downgrade, no underflow death.

The lifecycle also amends the trigger-semantics and durable-reports
decisions. The suppression gate in ADR 0008 becomes mode-aware: a
fixed entry is alive until its expiry, a dynamic entry while its value
stays above its death line, and Underflow re-arms writing exactly as
expiry and deletion do - the override gate (strictly higher recorded
volume) and the single-writer path are unchanged. ADR 0002's "reports
must survive restarts" narrows to fixed reports: the daemon sweeps
every dynamic report at startup, before the live-report set is
rebuilt, so no Report outlives the volatile tracker state that
justifies it. The restart amnesty is the deliberate price of that
volatility and is on record in ADR 0002: re-flagging after a restart
re-accumulates past volume from an empty bucket, so a burst-then-pause
offender gains amnesty from a restart during the pause, and a crash
loop is an amnesty machine - accepted because restarts are seldom, and
the mitigation if practice shows otherwise is seeding trackers from
report snapshots (the explicitly rejected fallback), not more report
state.

Considered: a "true" grace window counted as time sustained below the
line (rejected: identical to the margin under linear decay except in
the trickle-hover band, at the cost of lifecycle state on the record);
anti-flap windows beyond the margin (rejected: the margin is the whole
anti-flap story); a timer per dynamic report (rejected: the document
and the sweep see live tracker state anyway, so laziness costs
nothing). Consequences: a dynamic entry is marked in the report
document (`dynamic: true`, `expires_at: null`, `version` 0 additive)
and a dead one is simply absent from it - downstream withdrawal is
absence at the next sync, no new consumer handling. `ttl` and `grace`
are mutually exclusive at load, and a report action must carry `ttl`
on a tracker-less rule (a tripwire report can never be unbounded) and
on a zero-leak tracker (it can never underflow). Downgrades and
dynamic deaths log at `debug`; the startup sweep logs one summary
line.
