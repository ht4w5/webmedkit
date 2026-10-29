# 01: The per-direction enforcement scopes

**What to build:** An operator configures the ingress scope
(`--nft-ingress-daddr`, `--nft-ingress-dport`, `--nft-ingress-l4proto`)
and the egress scope (`--nft-egress-saddr`, `--nft-egress-sport`,
`--nft-egress-l4proto`) — comma lists or repeats, bare addresses as
host prefixes, protocols defaulting to `tcp,udp` — and both ban
directions enforce correctly. Ingress rules match their scope as
destination fields and the Target as source; egress rules match their
scope as source fields and the Target as destination; the protocol gate
is always present; a ban rule's family reachability follows the
installing direction's scope alone; either scope change re-enforces;
and the old shared-scope flags are refused. A port-forwarded service
(configured with its pre-translation view on the ingress scope and its
post-translation view on the egress scope) bans and meters on both
sides of the translation.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The six scope flags exist, named by the packet fields their
      direction matches; values accept comma lists and repeats;
      bare addresses count as their host prefixes; ports are exact
      numbers 1–65535; protocols are strict lowercase `tcp`/`udp`
      defaulting to `tcp,udp`
- [ ] The kernel encoding matches the split: ingress rules lead with
      the ingress scope at destination fields, egress rules with the
      egress scope at source fields, each direction's values distinct
      in the encoding tests
- [ ] The protocol gate is present in every kernel rule from its
      direction's scope (an unscoped ban covers tcp and udp traffic
      only)
- [ ] A ban rule's family reachability is decided by the installing
      direction's scope alone — an ingress-only scope never constrains
      egress rules and vice versa
- [ ] A change to either scope re-enforces (the ruleset hash folds both
      scopes)
- [ ] The old shared-scope flags are refused as unknown flags
- [ ] Refused flag values fail before enforcement, each error naming
      its flag and the offending value (protocol, port range, address
      form)
