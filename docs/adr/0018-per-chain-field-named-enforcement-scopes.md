# Per-chain, field-named enforcement scopes

The nftables driver's enforcement scope is no longer one shared
"service prefilter" of destination ports and addresses: each ban chain
carries its own scope, and the flags name the packet fields that chain
matches — `--nft-ingress-daddr/-dport/-l4proto` and
`--nft-egress-saddr/-sport/-l4proto`. ADR 0012's "service prefilter of
destination ports and addresses" is superseded **in part** here.

Because NAT translates a service's on-the-wire identity between hooks —
a request is still pre-translation at prerouting raw, its response is
still post-translation at forward — the two chains see one conversation
as different field values, and a single service-level scope cannot
describe both sides. The shared prefilter matched nothing on one side
of any DNAT'd service, silently. Field-named scopes make each chain's
view explicit: the operator states what each hook actually sees.

Settled with it: the tcp/udp gate always applies (default `tcp,udp`),
so an unscoped ban covers TCP/UDP only; scope flags take comma lists or
repeats, values are deduplicated and overlapping address prefixes are
refused at startup; a ban rule the scope cannot reach at all declines
loudly instead of vanishing from the plan; the `--nft-service-*` flags
are removed outright. Considered: one service-level scope plus
pre-translation view flags (rejected: two vocabularies for one
conversation); encoding both views as a union in the shared sets
(rejected: operator folklore, over-broad matches); moving the ingress
chain past the translation (rejected: bans must run before conntrack at
prerouting). Recorded because "which fields does this hook see?" is the
question the flags now force operators to answer — and the answer
differs for the two directions of the same service.
