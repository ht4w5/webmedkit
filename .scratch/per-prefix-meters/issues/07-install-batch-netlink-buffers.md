# 07: Install batch exceeds netlink wire limits (ENOBUFS / element corruption)

**What to build:** Fix the production failure `startup test install:
install: point current: apply: netlink receive: recvmsg: no buffer
space available` reported against the merged meters work (609 reports
compiling to 4 ban rules with per-prefix Meters), and the latent
element-list corruption found while minimising. Two defects, one
theme - the one-batch install runs into netlink wire limits:

1. The install batch is one netlink transaction whose every message
   is acknowledged; the ACK stream overflows the socket's receive
   buffer at a few hundred messages and the batch datagram exceeds
   the send buffer at a few thousand. Per-prefix Meters issue ~2
   messages per effective Target prefix, so real rulesets crossed the
   cliff.
2. One NEWSETELEM packs a whole element list into a single netlink
   attribute whose length is 16-bit: past ~64 KiB the length wraps,
   the kernel garbage-parses the tail (ENOENT) or silently truncates
   the set. Pre-existing (drop-rule interval sets too), latent at
   ~900 prefixes per set.

**Blocked by:** None (production incident).

**Status:** resolved

- [x] The netlink socket is sized at the driver's dial so one install
      batch and its acknowledgements fit (FORCE with a plain capped
      fallback), and the residual failure names the cause.
- [x] Set elements travel in wire-sized chunks (several NEWSETELEM
      messages in the same atomic batch).
- [x] Regression pins at the real-kernel seam: one above the buffer
      cliff (~90 prefixes before the fix), one asserting a large
      target list lands whole (silently truncated to 1179/3000 before
      the fix).

## Answer

Diagnosed with a driver-level feedback loop (root-gated Apply of N
metered prefixes in a scratch namespace) reproducing the user's exact
error at N=90. The ACK cliff and the send cliff are the same defect
family: the batch, its messages and its per-message acknowledgements
must fit fixed kernel socket buffers. A differential against the
pre-meters baseline showed the regression: element lists packed into
one message kept the pre-meters batch at a handful of messages, while
each Meter contributes a chain and a chain rule per prefix.

Fixes, in `splint/internal/firewall/nft/`:

- `netlinkBuffers` (via `nftables.WithSockOptions`) sizes the socket
  with SO_RCVBUFFORCE/SO_SNDBUFFORCE at 32 MiB requested (CAP_NET_ADMIN
  - which the daemon already needs to program nftables), falling back
  to the kernel-capped plain options, best effort. The Flush failure
  now names the cause when the limits are still exceeded. Validated
  to 3000 metered prefixes on the fallback path.
- `addSet` chunks element lists at 128 per NEWSETELEM through
  `SetAddElements`, keeping every message under the 16-bit attribute
  length; atomicity is unchanged (one batch). The fake kernel gained
  `SetAddElements` and the batch-shape pin now shows one
  `add-set-elems` per `add-set`.

Prior art: the fake-kernel shape pins and the root-gated
install/read-back suite. Mechanism details and cliff measurements in
the merge history (commits reference this ticket).

## Comments

Filed from a production run against the per-prefix meters feature
(2026-10-31). Defect 2 predates the meters work (drop sets with ~900+
prefixes were silently truncated); the meters' growth in messages
moved the reported ENOBUFS cliff within reach of real rulesets.
