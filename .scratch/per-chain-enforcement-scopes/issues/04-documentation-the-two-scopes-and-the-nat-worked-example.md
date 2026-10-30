# 04: Documentation: the two scopes and the NAT worked example

**What to build:** An operator with a NAT'd service configures both
scopes from the documentation alone. The enforcement-scope
documentation is rewritten in the glossary's vocabulary (enforcement
scope, ingress, egress), with a worked port-forward example showing the
pre-translation view on the ingress scope and the post-translation view
on the egress scope, the unscoped-ban tcp/udp delta, and a migration
note from the removed shared-scope flags. The usage block matches the
built help verbatim.

**Blocked by:** 01, 02, 03

**Status:** resolved

- [x] The enforcement-scope section describes the two scopes in
      glossary terms and states which packet fields each direction
      matches
- [x] The worked port-forward example carries the pre-translation and
      post-translation views on the right scopes
- [x] The unscoped-ban tcp/udp-only delta and the migration note from
      the removed flags are stated
- [x] The usage block is regenerated verbatim from the built help

## Comments

Resolved on branch `ticket/04-documentation`. What landed:
docs/splint.md's enforcement-scope section is rewritten around the two
scopes in glossary terms (enforcement scope, ingress, egress; no
"prefilter" in prose) with a per-direction table of the packet fields
each matches (destination fields on ingress, source fields on egress,
scope first, then the Target match), the value grammar and refusals
(comma lists/repeats, host prefixes, dedup, overlap refusal with the
run-captured error line), and the scope-unreachable decline; the
worked port-forward example is the centerpiece - containerized web
server on a docker bridge behind firewalld forward-port, docker
iptables disabled - carrying the pre-translation view at prerouting
raw on the ingress scope (`203.0.113.10:443`) and the post-translation
view at forward on the egress scope (`172.17.0.2:8443`), with a
run-captured dry-run plan (scope lines, placements, hash); the
unscoped-ban tcp/udp-only delta gets its own subsection; a migration
note maps the removed `--nft-service-*` flags onto both scopes (run
captured `unknown flag:` exit-2 line, mirror table, NAT caveat) and
lists the behavioral deltas. The usage block and every dry-run example
(scope lines, new hashes) are regenerated verbatim from `make build &&
bin/splint` runs against a scratch report endpoint; the cycle-log
hashes were refreshed to the same runs so the dry-run/hash
correlation the doc promises still holds. Driver options table,
penalty mapping, audit recipe and drivers intro updated for the six
flags and the two-scope wording. Docs-only: `make build && make test`
pass unchanged.
