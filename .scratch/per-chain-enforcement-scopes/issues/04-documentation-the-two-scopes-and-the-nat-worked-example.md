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

**Status:** ready-for-agent

- [ ] The enforcement-scope section describes the two scopes in
      glossary terms and states which packet fields each direction
      matches
- [ ] The worked port-forward example carries the pre-translation and
      post-translation views on the right scopes
- [ ] The unscoped-ban tcp/udp-only delta and the migration note from
      the removed flags are stated
- [ ] The usage block is regenerated verbatim from the built help
