# 03: The plan and logs tell the truth about scopes

**What to build:** "Why isn't this banned?" is answerable from the
plan and the logs alone. A ban rule the scopes cannot reach at all is
declined loudly — logged and counted with its reason, like every other
decline — instead of vanishing; a ban rule that only partially fits a
scope installs its reachable families with visibly narrowed Targets and
produces no decline; and `--dry-run` prints one scope line per
direction alongside the placement lines, so a pasted plan shows the
whole enforcement surface.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] A ban rule unreachable through its direction's scope becomes a
      decline carrying a reason, logged and counted like the
      request-rate decline
- [ ] A partially reachable ban rule installs its reachable families
      with narrowed Targets and is never declined
- [ ] The dry-run plan prints each direction's effective scope
      (protocols, ports, addresses) next to the placement lines
