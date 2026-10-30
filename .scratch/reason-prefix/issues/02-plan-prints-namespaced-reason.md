# 02: The `bandage -t` plan prints the namespaced reason

**What to build:** The operator validating a rules file sees the reason a
report will carry: every report action line in the `bandage -t` plan prints
`<program name>.<code>`, composed from the build identity the plan printer
already receives - so a renamed binary's plan shows its own namespace. The
plan's meaning is that what the operator approves is what ships. Rules loading
is unchanged and keeps its validation convention at this seam: a bare code
passes, a dotted code is refused. The rules documentation explains once that
codes are authored bare and carried namespaced, and its plan excerpts show the
namespaced form.

**Blocked by:** 01 (the plan previews "the report's reason", defined there)

**Status:** resolved

- [x] Every `report` action line in the plan prints the namespaced reason,
      matching the reason the report would carry
- [x] The plan's namespace follows the build identity the plan receives
- [x] Load validation at the plan seam keeps the failing/passing convention:
      a bare code passes, a code containing a dot is refused
- [x] Rules documentation: plan excerpts show the namespaced reason, and one
      prose note states codes are authored bare and carried namespaced
- [x] The plan-contract wording states that the report carries the code
      namespaced, while the rules file declares it bare

Prior art for the tests: the shipped-rules plan assertions and the rules
loader suite's one-failing-one-passing-case-per-validation convention.

## Answer

Implemented on integration branch `feat/reason-prefix` (commits
`1e82e8c`, `fe6f448`, `d158751`, merge `bbf9f0e`; cross-ticket
code-review fix `d730581`, merge `b0eb730`). Every `report` action
line in the `bandage -t` plan -- rule actions and ladder rungs alike --
prints `<program name>.<code>`, composed through the same
`domain.NamespacedReason` join the report uses, from the build identity
the plan printer already receives: `TestPlanReasonsFollowThePassedBuild`
pins `identity.For("bandage-canary")` -> `bandage-canary.<code>`, so a
renamed binary's plan shows its own namespace. Load validation at the
plan seam keeps the convention in `TestPlanReasonValidation`: a bare
code passes and the plan prints it namespaced, a code containing a dot
is refused with the authored value quoted. `docs/rules.md`'s plan
excerpts show the namespaced form (kept in lockstep with the
shipped-rules fence pin) and the `reason` bullet gained the one prose
note that codes are authored bare and carried namespaced; the
plan-contract wording on `app.Test` states the report carries the code
namespaced while the rules file declares it bare. Rules loading, rules
files and their bare codes are untouched.

Context: spec `../spec.md`, `bandage/internal/app/test.go`,
`bandage/internal/app/rules_test.go`,
`bandage/internal/app/shippedrules_test.go`, `docs/rules.md`, ADRs
`docs/adr/0016-report-reasons-are-namespaced.md` and
`docs/adr/0017-reason-prefix-comes-from-the-progname.md`.

Notes:
- The plan-contract wording lives in `Test`'s doc comment (the plan
  seam's contract).
- Ticket 01's loader-table dotted-refusal case and the plan-seam
  failing/passing pair overlap by design: two existing seams, one
  ticket each (kept per review disposition O1).
