# 08: The egress prefilter matches the service end

**What to build:** The enforcement scope's matches must follow the
conversation, not one packet's field names. The egress chain meters
the response direction (service → banned client), where the service
port and address appear as the packet's source - so the prefilter
must match sport/saddr there, mirroring the target match (source on
ingress, destination on egress). Before this fix both prefilter
matches were direction-agnostic: the port set matched the transport
header's destination port (offset 2) and the service-address set
matched the destination address in both chains. On egress that meant
the port set matched the client's ephemeral port and the address set
matched the banned client's own address against the service set -
`limit-bandwidth` rules could essentially never fire under a non-empty
`--nft-service-port`/`--nft-service-address`, and misfired when a
client's source port collided with a service port. The encoding-shape
tests asserted the prefilter only on ingress rules, which is why it
survived.

**Blocked by:** None

**Status:** resolved

- [x] `addBanRule` derives the prefilter offsets from the direction:
  dport/daddr on ingress, sport/saddr on egress (the service end -
  the endpoint opposite the target match)
- [x] The scoped kernel-test fixture carries a `limit-bandwidth` rule
  and asserts its egress shape: port load at transport offset 0,
  service-address load at the source offset, target at the
  destination offset
- [x] `Prefilter`'s doc comment and `docs/splint.md` state the
  request/response mirroring explicitly

Found in review of the nft prefilter logic. `Prefilter`'s
`DstPorts`/`DstAddrs` names keep describing the request direction
(the flags say "service destination port/address"); the driver
matches the same scope as source fields in the response direction.
