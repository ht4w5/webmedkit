# 03: Grace margin on dynamic reports

**What to build:** A rules author adds `grace: <duration>` (Go
duration strings plus `d` for days, like `ttl`) to a dynamic report
action, and the Report's death line widens from `volume` to
`volume − leak × grace`: a brief pause in otherwise sustained abuse no
longer flaps enforcement. `ttl` and `grace` on one report action is a
load error. The validated plan prints `grace=` beside `ttl=`. Absent
`grace` keeps the strict boundary.

**Blocked by:** 02

**Status:** resolved

- [x] A dynamic Report with `grace` stays alive while its value is
      above `volume − leak × grace` and dies at that line (strict).
- [x] Without `grace`, the strict line is unchanged (dead at
      `value <= volume`).
- [x] Load refuses a report action carrying both `ttl` and `grace`.
- [x] `grace` parses the `ttl` quantity language including day
      suffixes; unparseable values fail load naming the key.
- [x] The plan prints `grace=` on the report line.
- [x] Anti-flap is verifiable on the daemon seam: drive the value
      below the volume but above the widened line (clock advance),
      assert the Report survives; cross the line, assert death.

Implemented on branch `ticket/03-grace-margin` (behavior, validation
and plan in `7dac5ad`), targeting integration branch
`integration/dynamic-reports`. Grace compiles at rule build to a margin
of `leak × grace` on the report action and is recorded on the Report
(persisted in the row body, not the report document - checklist-silent,
taken from the spec's Report action grammar and "making the margin
available" to 04's downgrade line); every liveness act dies at
`volume − margin` through `Record.Underflow`'s death-line first arg.
On the plan line `grace=` prints beside `ttl=` on every report action,
`grace=none` when absent - mirroring `ttl=none`, so the pinned plan
expectations and the rules-guide fences moved in lockstep. The
no-grace strict line stays pinned by the unchanged
`TestE2EDynamicReportDiesAtTheStrictBoundary`. Downgrade (04) and the
startup sweep (05) are out of this ticket.
Context: spec `../spec.md` (Implementation Decisions - Report action
grammar, Dynamic report liveness; Further Notes - Grace versus
margin).
