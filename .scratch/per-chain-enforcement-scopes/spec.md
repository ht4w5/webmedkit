# splint: per-chain enforcement scopes — spec

Status: ready-for-agent

## Problem Statement

The nftables driver scopes every ban to "service traffic" with one
shared pair of destination-only sets — the `"service prefilter"` of
destination ports and addresses. But the two ban directions see one
service as *different packet fields*: the ingress (the traffic a ban
rule's Target sends) carries the service at its destination fields,
while the egress (the response direction) carries the same service at
its source fields. Where NAT translates the service's on-the-wire
identity between hooks — a port-forwarded service is the everyday case
— the shared prefilter can only describe one side. The other side
matches nothing, silently: reject bans never fire, or bandwidth meters
never fire, and the counters stay at zero with nothing in the logs to
explain it.

Around that, the flag surface is clumsy: values are repeat-only (no
comma lists), the transport protocol set is hard-coded to tcp and udp
with no way to narrow it, duplicate values can fail a kernel batch, and
a ban rule the scope cannot reach at all simply vanishes from the plan
— `--dry-run` never mentions it, so "why isn't this banned?" has no
answer.

## Solution

Two **enforcement scopes** — the *ingress scope* and the *egress
scope* — each named by the packet fields its direction matches:
`--nft-ingress-daddr/-dport/-l4proto` and
`--nft-egress-saddr/-sport/-l4proto`. The operator states exactly what
each hook sees; a NAT'd service configures its pre-translation view on
the ingress scope and its post-translation view on the egress scope,
and both directions enforce. Values accept comma lists or repeats,
protocols default to `tcp,udp`, everything is validated and deduplicated
at startup, unreachable ban rules decline loudly with a reason, and the
dry-run prints each direction's effective scope so the plan is
self-contained. The old shared-scope flags are removed outright.

## User Stories

1. As a splint operator running a port-forwarded service, I want the
   enforcement scope split by direction and named by the packet fields
   each direction matches, so that every scope states exactly what its
   hook sees and bans work on both sides of the translation.
2. As a splint operator, I want an ingress scope matched as
   destination address, destination port and protocol, so that drop,
   reject and packet-rate penalties match my service's request
   direction.
3. As a splint operator, I want an egress scope matched as source
   address, source port and protocol, so that bandwidth penalties meter
   my service's response direction.
4. As a splint operator, I want scope values accepted as comma lists,
   so that a service's ports and addresses go in as one flag value.
5. As a splint operator, I want scope flags to still accept repeats,
   so that my command-line habits keep working.
6. As a splint operator, I want addresses accepted as bare addresses or
   prefixes, so that single hosts need no hand-written host prefix.
7. As a splint operator, I want the protocol scope to default to
   `tcp,udp`, so that the common case needs no flag at all.
8. As a splint operator, I want to scope a direction to `tcp` or `udp`
   alone, so that a single-protocol service is scoped exactly.
9. As a splint operator, I want protocol values outside `tcp` and `udp`
   refused at startup, so that a typo can never silently widen or empty
   a scope.
10. As a splint operator, I want port values validated to 1–65535, so
    that bad values fail before anything is enforced.
11. As a splint operator, I want duplicate scope values deduplicated,
    so that repeated or overlapping list entries never fail inside a
    kernel batch.
12. As a splint operator, I want overlapping address prefixes refused
    at startup with a clear error, so that my mistakes surface before
    enforcement, not inside it.
13. As a splint operator, I want every refused value reported with its
    flag name and the offending value, so that fixing the command line
    is immediate.
14. As a splint operator, I want a ban rule the scope cannot reach at
    all declined loudly with a reason and counted, so that "why isn't
    this banned?" is answerable from the logs.
15. As a splint operator, I want a ban rule that only partially fits
    the scope to install for the address families it can reach, so that
    mixed-family Targets still enforce where the scope can see them.
16. As a splint operator, I want the dry-run plan to print each
    direction's effective scope alongside the placement lines, so that a
    pasted plan shows the whole enforcement surface.
17. As a splint operator, I want a change to either scope to re-enforce
    automatically, so that scope edits never leave stale rules behind.
18. As a splint operator, I want the old shared-scope flags removed
    outright, so that no flag surface can express the one-sided scope
    model again.
19. As a splint operator fanning out over network namespaces, I want
    one scope pair shared by all enforcement points, so that a
    multi-container service configures one union scope.
20. As a splint operator, I want the protocol default to apply even
    when no port or address is set, so that scope semantics are uniform
    across every flag combination (an unscoped ban covers tcp and udp).
21. As an incident responder, I want declines to carry their reason
    through the engine's logs and counters, so that a missing ban is
    diagnosed from logs alone.
22. As a web service operator, I want bans scoped to my service's
    protocols, ports and addresses, so that SSH, monitoring and
    unrelated forwarded traffic are never swept in.
23. As a legitimate user of the protected service, I want only the
    banned Target's service traffic affected, so that my access is
    untouched even when a broad prefix is banned nearby.
24. As a splint operator, I want per-ban-rule hit counters to keep
    mapping onto their ban rules across rebuilds, so that hit reporting
    is unchanged by the new scope model.
25. As a documentation reader with a NAT'd service, I want a worked
    port-forward example showing both scopes with their real on-the-wire
    views, so that I can map my own NAT setup onto the flags.
26. As a documentation reader upgrading, I want a migration note from
    the removed flags to the new ones, so that I can update my command
    lines confidently.
27. As a documentation reader, I want the enforcement-scope
    documentation to use the glossary's terms (enforcement scope,
    ingress, egress), so that flags, docs and ADRs speak one language.

## Implementation Decisions

- The nftables firewall driver's configuration carries two enforcement
  scopes — one per direction — each a protocol set, a port set and an
  address set. The shared "service prefilter" is removed; ADR 0018
  records the change and supersedes ADR 0012's phrasing in part.
- The driver's flag surface grows to six flags, named by the packet
  field each direction matches (destination fields for ingress, source
  fields for egress); the two shared-scope flags are removed with no
  alias or deprecation shim.
- Value grammar: comma lists and repeats both accumulate; addresses
  accept a bare address (its host prefix) or a prefix; ports are exact
  numbers 1–65535 (no ranges); protocols are strict lowercase `tcp` or
  `udp`, defaulting to `tcp,udp`.
- Startup validation deduplicates all values and refuses overlapping
  address prefixes; errors name the flag and the offending value.
  Refused values keep the configuration-error exit; unknown flags keep
  the refused-command-line exit.
- The protocol gate is always present in every kernel rule, drawn from
  its direction's scope; an unscoped ban therefore covers tcp and udp
  traffic only (a deliberate, documented delta).
- Rule encoding order: the direction's scope matches first (protocol
  gate, port, address), then the Target match (source on ingress,
  destination on egress), then the counter and the verdict. The scope
  matches the service end of the conversation — destination fields on
  ingress, source fields on egress.
- Per-ban-rule sets keep their ban-rule index in their names, so
  counter read-back mapping survives restarts; the scope address sets
  are renamed to reflect their role (they are no longer destination
  sets on every rule).
- Plan semantics: family reachability is decided by the installing
  direction's scope alone; a ban rule the scope cannot reach at all
  becomes a decline with a reason (logged and counted like other
  declines); a partially reachable ban rule installs its reachable
  families with narrowed Targets and stays out of the declines; the
  ruleset hash folds both scopes; the dry-run prints one scope line per
  direction.
- All enforcement points of one instance (the network-namespace
  fan-out) share the one scope pair; multi-container services pass
  unions of addresses and ports.
- Documentation: the enforcement-scope section is rewritten around the
  two scopes with the port-forward worked example as its centerpiece,
  plus the unscoped-ban delta and the migration note; the usage block
  is regenerated verbatim from the built help.

## Testing Decisions

- Good tests assert external behavior only: what the flag surface
  accepts and refuses, what the plan and outcomes say, and what the
  kernel ruleset looks like — never internal helpers or call shapes.
- Seams (all existing, none new): the driver factory's
  bind/validate/open surface for parsing and validation; the firewall
  driver port (plan/apply/reset) over the fake kernel for behavior; the
  real-kernel read-back for encoding shape, per the repo's convention
  that netlink encoding is verified only there. Parse helpers stay
  internal, covered through the factory surface.
- Prior art: the existing factory validation tests; the existing
  fake-kernel driver tests (plan derivation, skip-if-unchanged, counter
  read-back, decline reporting, atomic batch); the existing root-gated
  kernel tests with hand-written expected rule shapes.
- New cases: comma lists, repeats, bare addresses, protocol default and
  narrowings; refused values (bad protocol, port out of range, garbage
  address) each naming its flag; deduplication and overlap refusal;
  the always-present protocol gate; per-direction narrowing
  independence (an ingress-only scope never constrains egress rules and
  vice versa); scope-unreachable decline with reason; partial narrowing
  visible in the plan; hash change on either scope; dry-run scope
  lines; and at the kernel: ingress rules match destination fields
  while egress rules match source fields, with distinct scope values
  per direction so the split is pinned.

## Out of Scope

- Port ranges in the port scope.
- Per-enforcement-point (per-namespace) scope overrides.
- Protocol values beyond `tcp` and `udp` — no `any`, no `icmp`.
- Silently merging overlapping address prefixes.
- Deprecated aliases for the removed shared-scope flags.
- Any change to Penalty semantics (the `limit-req-rate` decline stands)
  or to hook placement (input/prerouting modes unchanged).
- The ban-rule Target merge policy (targets are structurally disjoint
  by construction) and the counter read-back mechanism.

## Further Notes

- The motivating deployment: a containerized web server on a docker
  bridge behind a host that port-forwards public ports to it with
  firewalld rich rules (docker's iptables integration disabled). The
  ingress direction sees the pre-translation view at prerouting raw;
  the egress direction sees the post-translation view at forward; the
  shared prefilter matched one side only. The worked example in the
  docs should carry exactly this shape.
- The unscoped-ban delta (tcp and udp only) is accepted; a later
  compatible widening could add an explicit protocol catch-all if a
  real need appears.
- The glossary terms (enforcement scope, ingress, egress) and ADR 0018
  are already recorded; the spec, flags and docs must use them.
