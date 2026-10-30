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

**Status:** resolved

- [x] A ban rule unreachable through its direction's scope becomes a
      decline carrying a reason, logged and counted like the
      request-rate decline
- [x] A partially reachable ban rule installs its reachable families
      with narrowed Targets and is never declined
- [x] The dry-run plan prints each direction's effective scope
      (protocols, ports, addresses) next to the placement lines

## Comments

Resolved on branch `ticket/03-plan-and-logs`. What landed: a ban rule
the installing direction's enforcement scope cannot reach at all is
now a `firewall.Decline` (feature = the stance, reason naming the
direction's scope, e.g. "the ingress enforcement scope cannot reach
any target address family") instead of a silent skip in
`nft.Driver.Plan` — it flows through the engine's existing warn log
and the `declined=N` count with zero engine changes; partial narrowing
keeps installing the reachable families with narrowed Targets and is
never declined (pinned in the plan-visible fake-kernel tests); and the
dry-run plan prints one already-formatted scope line per direction
next to the placement lines — `firewall.Plan` gained the opaque
`Scopes []string` surface (like `Placement`: printed and never
parsed), `firewalltest.Stub` gained `Scopes`, and the driver formats
`scope=ingress l4proto=… dport=… daddr=…` /
`scope=egress l4proto=… sport=… saddr=…` (field names matching the
flags, `all` for unscoped sets, the `tcp,udp` default always shown);
`app` still imports only the firewall seam (deps_test.go). The
per-direction independence subtests were refit so each fixture stays
truthful under the new decline semantics.
