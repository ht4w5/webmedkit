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

**Status:** resolved

- [x] Decide the exemption semantics (skip the rest of the chain vs. skip
      only covering bans) and where the "known limitation" note in the
      operator docs moves to.
- [ ] ~~Implement the chosen semantics in the nftables driver's meter chains.~~
      Cancelled: the chosen semantics is the status quo.
- [ ] ~~Extend the real-kernel suite with the carve-out scenario: a specific
      Meter under a broader drop; in-rate traffic passes, over-rate traffic
      drops.~~ Cancelled: the fall-through behavior is already pinned by
      ticket 03's traffic pins.
- [ ] ~~Update the operator documentation: the limitation section becomes a
      description of the chosen semantics.~~ Cancelled: the "known
      limitation" note in docs/splint.md already describes fall-through
      as the semantics.

## Answer

Maintainer decision (2026-06): **keep fall-through, for now.** In-rate
metered traffic continues to fall off the Meter chain into the rule
walk and stays subject to broader ban rules below - the documented
first-match limitation stands as the chosen semantics. No exemption
mechanism is built: neither chain-evaluation termination (which would
exempt the packet from *all* later rules) nor a scoped exemption.

Consequences: no code or docs change. The behavior is pinned by the
traffic pins on `feat/per-prefix-meters` (`TestKernelSaturatingOneMeterLeavesItsSiblingUnaffected`
uses the broader drop below as its fall-through witness), and the
"Known limitation: first-match meter stances" note in docs/splint.md
stays exactly where it is - its "fixing it is a driver-policy
question" framing remains accurate.

"For now" is deliberate: if an exemption is wanted later, reopen as a
new issue and re-answer the scoped-vs-unscoped question then. ADR 0020
records fall-through as preserved behavior, so this decision conflicts
with nothing.

## Comments

Filed from the per-prefix meters design interview (2026-06); the limitation
is pre-existing (documented in docs/splint.md) and deliberately out of scope
for the meters spec. Renumbered from 01 when implementation tickets landed
ahead of it.
