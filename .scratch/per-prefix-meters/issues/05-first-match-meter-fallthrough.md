# 05: Exempt in-rate metered traffic from broader bans (first-match limitation)

**What to build:** When a Meter carves an exception under a broader ban rule
(the merge policy puts the more specific Target first), in-rate traffic of
that Target is not exempted: it falls off the meter chain back into the rule
walk and the broader ban rule below still drops it. Fix so that in-rate
traffic of a metered prefix passes, while over-rate traffic is still dropped.
Note the open semantic question: the obvious mechanism (terminating the
packet's chain evaluation on the in-rate path) exempts the packet from *all*
later rules in the chain, not just the broader ban below - decide whether
that is acceptable or whether exemption must be scoped.

**Blocked by:** 04: Per-prefix Meters for limit-bandwidth - all Meter chains
land there; this changes their exit semantics.

**Status:** needs-triage

- [ ] Decide the exemption semantics (skip the rest of the chain vs. skip
      only covering bans) and where the "known limitation" note in the
      operator docs moves to.
- [ ] Implement the chosen semantics in the nftables driver's meter chains.
- [ ] Extend the real-kernel suite with the carve-out scenario: a specific
      Meter under a broader drop; in-rate traffic passes, over-rate traffic
      drops.
- [ ] Update the operator documentation: the limitation section becomes a
      description of the chosen semantics.

## Comments

Filed from the per-prefix meters design interview (2026-06); the limitation
is pre-existing (documented in docs/splint.md) and deliberately out of scope
for the meters spec. Renumbered from 01 when implementation tickets landed
ahead of it.
