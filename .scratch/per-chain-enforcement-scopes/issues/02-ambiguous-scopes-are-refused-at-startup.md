# 02: Ambiguous scopes are refused at startup

**What to build:** A scope list that is ambiguous never reaches the
kernel. Duplicate values are deduplicated silently; overlapping address
prefixes in one scope are refused at startup with an error naming the
flag and the offending value. Configuring
`--nft-ingress-daddr 10.0.0.0/8,10.0.0.0/16` exits before anything is
enforced, with a message precise enough to fix the command line
immediately.

**Blocked by:** 01

**Status:** resolved

- [x] Duplicate scope values (addresses, ports, protocols) are
      deduplicated before enforcement
- [x] Overlapping address prefixes within one scope are refused at
      startup, the error naming the flag and the offending values
- [x] A refused scope touches no enforcement surface: the refusal
      happens at startup, before any ruleset is installed

## Comments

Resolved on branch `ticket/02-ambiguous-scopes`. What landed: the
factory's `parseScope` now resolves each direction's scope as a set —
duplicate values (ports, protocols, addresses, including a bare
address and its host prefix) are deduplicated silently, keeping first
occurrences — and refuses any remaining overlapping address pair
within one scope (`netip.Prefix.Overlaps`) at startup with
`<flag>: overlapping address prefixes (got <a> and <b>)`, naming the
flag and both values. Equal prefixes are duplicates, not overlaps;
cross-scope overlap is fine (the scopes describe different
directions). The refusal runs in `Validate`, before any enforcement
point opens, so a refused scope touches no enforcement surface.
Tests at the factory bind/validate/open surface: dedup witnessed
through the ruleset hash (the resolved-scope witness), overlap
accept/refuse cases in the validation table, and an Open test pinning
zero enforcement points on a refused scope.
