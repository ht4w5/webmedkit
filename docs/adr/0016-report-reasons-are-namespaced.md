# Report reasons are namespaced

A report's `reason` — the machine-readable trigger code feeding the ban
rules downstream tooling generates from reports (ADR 0004) — is
**namespaced**. On the wire it is `<prefix>.<code>`, matching
`^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*$`: any prefix matching
`^[a-z0-9][a-z0-9_-]*\.` is legal for the report, and how a producer
chooses its prefix is deliberately not the report contract's business.
The rules DSL keeps taking bare codes (`reason:`, still
`^[a-z0-9][a-z0-9_-]*$`); bandage adds the prefix when it creates the
report, so the authored code is recoverable as the reason's second
label. The report document stays at `version` 0: a pre-release pattern
tightening, not a field or shape change.

Superseded in part ADR 0004's pattern clause: the lowercase slug
pattern stands for the authored code and for each label, but the reason
a report carries is no longer a single slug.

Considered: keeping the bare slug (rejected: an unnamespaced shared
vocabulary, where two producers' codes collide and a consumer cannot
attribute them); making the prefix a rule-author concern, written in the
rules file (rejected: the namespace belongs to the producer, not to each
rules file); pinning one producer's prefix in the contract (rejected:
the contract constrains the shape, not the label choice — bandage's own
choice is ADR 0017). Recorded because a dotted prefix on exactly one
field looks arbitrary until you see the collisions it prevents.

Consequences: a consumer matches on the full namespaced code or strips
the first label to recover the authored one; slug validation on the
consumer side must widen from one label to two.
