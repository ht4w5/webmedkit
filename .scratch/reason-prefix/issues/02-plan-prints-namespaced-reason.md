# 02: The `bandage -t` plan prints the namespaced reason

**What to build:** The operator validating a rules file sees the reason a
report will carry: every report action line in the `bandage -t` plan prints
`<program name>.<code>`, composed from the build identity the plan printer
already receives — so a renamed binary's plan shows its own namespace. The
plan's meaning is that what the operator approves is what ships. Rules loading
is unchanged and keeps its validation convention at this seam: a bare code
passes, a dotted code is refused. The rules documentation explains once that
codes are authored bare and carried namespaced, and its plan excerpts show the
namespaced form.

**Blocked by:** 01 (the plan previews "the report's reason", defined there)

**Status:** ready-for-agent

- [ ] Every `report` action line in the plan prints the namespaced reason,
      matching the reason the report would carry
- [ ] The plan's namespace follows the build identity the plan receives
- [ ] Load validation at the plan seam keeps the failing/passing convention:
      a bare code passes, a code containing a dot is refused
- [ ] Rules documentation: plan excerpts show the namespaced reason, and one
      prose note states codes are authored bare and carried namespaced
- [ ] The plan-contract wording states that the report carries the code
      namespaced, while the rules file declares it bare

Prior art for the tests: the shipped-rules plan assertions and the rules
loader suite's one-failing-one-passing-case-per-validation convention.
