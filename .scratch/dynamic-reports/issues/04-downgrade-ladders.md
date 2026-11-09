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

**Status:** resolved

- [x] A dynamic ban report (top rung) demotes to the throttle rung's
      stance as the value decays between the rungs, carrying the lower
      rung's reason and Penalty; the Sample backing each stance comes
      from the climb event.
- [x] `created_at` is frozen across downgrades; `updated_at` moves.
- [x] A downgrade onto a fixed rung stamps that rung's `ttl` from the
      write; the stance then behaves as a fixed report.
- [x] The Report dies when the value decays below the lowest rung
      (minus the stance's margin).
- [x] Fixed stances never downgrade or die at underflow, in single
      volume rules and ladder rungs alike.
- [x] The margin guards the downgrade line (no boundary flapping
      between rungs).
- [x] Downgrade/demote acts appear only at `debug` log level.
- [x] Mixed-ladder behavior is covered on the daemon seam: one ladder
      with a dynamic rung and a fixed rung, clock-driven decay asserted
      through the served document.

## Comments

Implemented on branch `ticket/04-downgrade-ladders` (behavior and
daemon-seam tests in `642798c`, the ladder ADR narrowing in `2f20dc1`),
targeting integration branch `integration/dynamic-reports`. The
Downgrade materializes lazily at document generation and at sweep time:
the report service resolves the restatement through a Ladder port (the
rule's report-carrying rungs with namespaced reasons, assembled from
the loaded chains), restate-or-die decided against the decayed value -
a rung remains exceeded while the value is strictly above `at - margin`
(the restated stance would be alive), so the report chains down to the
lowest rung's line and dies there, strict. The restatement lands
through the single-writer protocol and skips the round when the row
moved since it was read. "the Sample backing each stance comes from
the climb event" is read with the spec's "Sample ... move" clause: the
Sample and the reading's value keep the climb's (the evidence that
crossed every lower rung), only the reading's volume restates to the
target rung's threshold - checklist-silent, taken from user story 14.
A downgrade onto a fixed rung is served as a fixed entry (dynamic
marker gone, `expires_at` stamped from the write), so mixed ladders
behave per-rung in both directions. Downgrade acts log at `debug`
(checklist); dynamic deaths log at `debug` too where the row goes -
checklist-silent, taken from the spec's Observability decision, at
document generation, at sweep time and at the re-flag write - one
line per death act, where the row goes, never one per document.
Context: spec `../spec.md` (Implementation Decisions - Downgrade,
Dynamic report liveness, Trigger suppression; User stories 13-18).
