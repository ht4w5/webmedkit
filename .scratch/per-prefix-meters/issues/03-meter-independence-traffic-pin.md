# 03: Kernel traffic pin - Meter independence and fall-through

**What to build:** Behavioral proof against a real kernel that per-prefix
Meters really are independent - the regression pin against silent budget
sharing creeping back. A scratch-namespace harness drives real packets
through the ingress hook: traffic saturating one limited prefix at its
stated rate gets its excess dropped while a same-rate sibling prefix is
unaffected, and in-rate traffic of both prefixes falls through as today.
This is the repo's first traffic-driving kernel test; it must stay safe next
to a live deployment like the existing namespace tests.

**Blocked by:** 02: Per-prefix Meters for limit-pkt-rate

**Status:** ready-for-agent

- [ ] A root-gated test drives packets through the enforcement hook of a
      scratch namespace (no test can touch a live deployment's state).
- [ ] Saturating one prefix's Meter drops its excess while a same-rate
      sibling prefix's traffic passes unaffected.
- [ ] In-rate traffic of a metered prefix falls through unchanged.
- [ ] The pin is read through the same hit/counter read-back operators use,
      so it exercises the full path, not just kernel internals.
