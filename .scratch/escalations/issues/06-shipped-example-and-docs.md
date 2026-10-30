# 06: Shipped ladder example and reference docs

**What to build:** The shipped rules config gains a worked escalation
ladder - handoff rung, throttle recommendation, ban - that validates,
prints its whole climb in `bandage -t`, and stands as the reference
example operators copy. The rules guide's escalation section completes
(worked example, per-kind quantities, per-fired-rung terminality, the
in-place override and its freeze rules, the jump-not-goto handoff idiom),
and the rules format and validation reference sections describe the
ladder and its load-time matrix, so the documentation set and the
implementation cannot drift.

**Blocked by:** 04, 05

**Status:** resolved

- [x] The shipped rules config carries a worked ladder (handoff →
      throttle → ban) that loads and validates
- [x] `bandage -t` on the shipped config shows every rung with its
      threshold and actions
- [x] The rules guide's escalation section is complete with the worked
      example and the handoff idiom
- [x] The rules format and validation reference sections describe the
      ladder, the per-kind threshold quantities and the full load-time
      validation matrix
- [x] No documented behaviour in the set describes anything the
      implementation does not do, and vice versa

Implemented on integration branch `feat/escalations` (commit `19ef271`;
cross-ticket code-review fix pass through `129c5ea`). The shipped
`configs/rules.yml` carries the `iso-egress-ladder` worked ladder
(handoff `jump` at 50MB, throttle at 5GB, ban at 40GB) plus its
`inspect` handoff chain, pinned through the loader and plan seams in
`shippedrules_test.go`. The rules guide's escalation section is
complete (worked example, per-kind threshold quantities, per-fired-rung
terminality, the in-place override and its freeze rules, the
jump-not-goto idiom) and the rules format and validation reference
sections describe the ladder and the full load-time matrix; a drift
audit across the rules guide, report contract, PLAN.md and GLOSSARY.md
closed every divergence in both directions.

Context: spec `../spec.md` (Modules touched - documentation set);
completes the sections tickets 03-05 opened.
