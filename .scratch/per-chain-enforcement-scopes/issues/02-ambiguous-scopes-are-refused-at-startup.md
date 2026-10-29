# 02: Ambiguous scopes are refused at startup

**What to build:** A scope list that is ambiguous never reaches the
kernel. Duplicate values are deduplicated silently; overlapping address
prefixes in one scope are refused at startup with an error naming the
flag and the offending value. Configuring
`--nft-ingress-daddr 10.0.0.0/8,10.0.0.0/16` exits before anything is
enforced, with a message precise enough to fix the command line
immediately.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Duplicate scope values (addresses, ports, protocols) are
      deduplicated before enforcement
- [ ] Overlapping address prefixes within one scope are refused at
      startup, the error naming the flag and the offending values
- [ ] A refused scope touches no enforcement surface: the refusal
      happens at startup, before any ruleset is installed
