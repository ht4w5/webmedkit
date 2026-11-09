# 04: Downgrade through ladders

**What to build:** A dynamic ladder Report decays *down* its ladder
instead of vanishing at the top rung's line: as the value falls below
the current stance's death line (margin-guarded, no rung flapping at
the boundary), the Report restates itself to the highest rung still
exceeded - reason, Penalty, Sample and `updated_at` move to that
rung's values, `created_at` stays frozen - and adopts the target
rung's policy: a fixed rung's `ttl` runs from the downgrade write, a
dynamic rung keeps value-bound liveness. The Report dies when no rung
remains exceeded. Fixed stances never decay-interact (no downgrade,
no underflow death). Mixed ladders behave per-rung. Downgrades log at
`debug` only. The ladder ADR's "no downgrade and no rewrite" narrows
to "no downgrade from Triggers"; decay-driven Downgrade is its
inverse.

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] A dynamic ban report (top rung) demotes to the throttle rung's
      stance as the value decays between the rungs, carrying the lower
      rung's reason and Penalty; the Sample backing each stance comes
      from the climb event.
- [ ] `created_at` is frozen across downgrades; `updated_at` moves.
- [ ] A downgrade onto a fixed rung stamps that rung's `ttl` from the
      write; the stance then behaves as a fixed report.
- [ ] The Report dies when the value decays below the lowest rung
      (minus the stance's margin).
- [ ] Fixed stances never downgrade or die at underflow, in single
      volume rules and ladder rungs alike.
- [ ] The margin guards the downgrade line (no boundary flapping
      between rungs).
- [ ] Downgrade/demote acts appear only at `debug` log level.
- [ ] Mixed-ladder behavior is covered on the daemon seam: one ladder
      with a dynamic rung and a fixed rung, clock-driven decay asserted
      through the served document.
