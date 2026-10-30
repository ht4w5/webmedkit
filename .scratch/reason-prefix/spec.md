# Report reasons carry a producer namespace - spec

Status: ready-for-agent

## Problem Statement

A report's Reason is the machine-readable trigger code that feeds the ban
rules downstream tooling generates from reports - the one field of the report
that exists to cross tool boundaries. Today it is the bare code the rule author
wrote (`egress-overflow`, `ua-trip`). Those codes live in a shared, unnamespaced
vocabulary: when a consumer sees `ua-trip` in a ban rule, it can tell neither
which tool produced the code nor whether two tools meant the same thing by it,
and two producers can collide on the same code with different meanings. The
compatibility contract ADR 0004 pinned (a single lowercase slug) has no room
for attribution.

## Solution

A report's Reason is **namespaced**: on the wire it is `<prefix>.<code>`,
matching `^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*$`. The contract constrains
the shape - any prefix matching `^[a-z0-9][a-z0-9_-]*\.` is legal - and not any
producer's naming choice. bandage composes its prefix from its own program name
plus a dot, so the namespace follows the tool automatically through any rename,
and adds it at the moment a report is created. Rule authors keep writing bare
codes in their rules files exactly as before; what a report carries, stores, and
serves is the namespaced form. The report document stays at version 0: this is
a pre-release pattern tightening, and the authored code remains recoverable as
the reason's second label.

## User Stories

1. As a report consumer, I want every report's Reason to be namespaced as
   `<prefix>.<code>`, so that I can attribute a trigger code to its producer
   without a shared registry.
2. As a report consumer, I want the wire pattern published in the report
   document contract as `^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*$`, so that
   I can validate documents against a stated pattern.
3. As a report consumer, I want any prefix matching `^[a-z0-9][a-z0-9_-]*\.`
   to be legal for a report, so that the contract constrains the shape and
   never a producer's naming choice.
4. As a report consumer, I want to recover the authored code by stripping the
   reason's first label, so that my existing per-code logic keeps working on
   the second label alone.
5. As a report consumer, I want the report document to stay at version 0, so
   that the contract tightens pre-release without a version migration on my
   side.
6. As a report consumer, I want the canonical example in the contract to show
   a namespaced reason, so that the byte-pinned example demonstrates the form
   I must accept.
7. As a report consumer, I want the reason's other properties preserved - it
   is still non-unique, and several rules may still share one code so multiple
   signals feed a single ban reason, so that namespacing changes only the
   code's form.
8. As a report consumer, I want the namespaced reason identical in storage and
   on the wire, so that what I fetch is exactly what was written.
9. As a report consumer, I want a report overridden by a higher escalation to
   carry the overriding rung's namespaced code, so that the report's current
   stance and its code always come from the same rung.
10. As a rule author, I want my `reason:` key to keep taking the bare code, so
    that no rules file changes and the namespace is not mine to maintain.
11. As a rule author, I want bandage to refuse any `reason:` value that is not
    a bare single-label slug, so that a prefix can never be faked, doubled, or
    smuggled into my configuration.
12. As a rule author, I want the `bandage -t` plan to print the namespaced
    reason, so that I audit exactly what report consumers will see.
13. As a rule author, I want the plan and the report to show the same reason
    string, so that what I approve before deploying is what ships.
14. As a rule author, I want the rules documentation to explain once that
    codes are authored bare and carried namespaced, so that I am not surprised
    by the prefix in reports.
15. As an operator, I want the prefix derived from the daemon's program name,
    so that renaming the binary re-namespaces every reason with a single edit
    and the tool's names stay in step.
16. As an operator, I want a program rename to change reason values without
    breaking conformance or bumping the document version, so that a rename is
    a tooling concern, not a wire migration.
17. As a future producer of reports or ban rules, I want my own prefix legal
    under the same published pattern, so that I can ship tooling against
    reports without coordinating naming with bandage.
18. As a report consumer, I want bandage's own prefix to be documented as a
    producer-side choice outside the contract, so that I never depend on its
    literal first label.
19. As a maintainer, I want the program name constrained to the slug shape,
    so that the composed prefix always matches the contract pattern.
20. As a maintainer, I want that constraint pinned by a test where the prefix
    is composed, so that a malformed rename fails the test suite rather than
    a running daemon.
21. As a maintainer, I want the prefix composed from the build identity the
    daemon already receives, so that the domain core stays dependency-free and
    no new shared package exists.
22. As a maintainer, I want the namespace decision and the prefix-source
    decision recorded as separate ADRs, so that the contract boundary between
    them survives future reorganizations.
23. As a maintainer, I want the glossary's Reason entry to describe the
    namespaced form without pinning any producer's spelling, so that the
    vocabulary stays true for every producer.

## Implementation Decisions

- A report's Reason becomes the namespaced form `<prefix>.<code>`, where the
  prefix (with its dot) and the code each match the single-label slug pattern
  `^[a-z0-9][a-z0-9_-]*$`; the full wire pattern is
  `^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*$` (ADR 0016).
- The prefix is composed where the report is created - the report action
  being the only write path to the report table - and nowhere else: not at
  rules load, not at serialization. The report action keeps carrying the
  author's bare code; the created report carries the namespaced form.
- Composition is a plain join of the injected namespace and the code; no
  defensive re-prefixing. The load-time validator already refuses dotted
  codes, so double prefixes cannot occur.
- The rules DSL contract is unchanged: `reason:` takes a bare single-label
  slug under the same pattern as before, and the loader's refusal messages
  keep quoting the authored value.
- bandage's prefix is its program name plus a dot (ADR 0017): the program
  name is a single constant in the analyzer's command, reaching report
  creation through the build identity the daemon and the plan printer already
  receive. The domain core remains stdlib-only; its dependency lint is
  untouched and no shared constant package is introduced.
- The program-name constraint (single-label slug shape) is documented and
  test-pinned, not validated at startup.
- The report document stays at `version` 0. The contract states the generic
  two-label pattern and pins the current literal only in its example; how
  bandage chooses its prefix is explicitly outside the contract.
- The `bandage -t` plan renders the namespaced reason, computed from the same
  build identity the plan already receives, so a renamed binary's plan shows
  its own namespace.
- Persistence stores the reason as the namespaced form; there is no read-path
  normalization and no migration (no production data exists).
- Report identity, sharing of codes across rules, escalation override
  semantics, and every other report field are unchanged.
- The consumer binary needs no change: its wire model carries the reason
  pass-through and ban-rule compilation reads only target and penalty.
- Documentation updated: the report document contract (field row, pattern,
  canonical example), the rules documentation (plan excerpts and one prose
  note that codes are authored bare and carried namespaced), the glossary's
  Reason entry, and ADRs 0016/0017 with a superseded-in-part note on ADR 0004
  (its authored-slug pattern stands; its single-slug wire pattern does not).

## Testing Decisions

A good test here asserts only external behavior at the highest seams: what a
served report carries and what the plan prints - never where the composition
happens or what helper does it. The feature is specified at three existing
seams; no new seams are introduced:

- **The driven daemon observed through the `GET /reports` document** (the
  highest seam): a created report carries `<prefix>.<code>`; an escalation
  override carries the overriding rung's namespaced code; the served document
  reproduces the canonical example byte for byte. Prior art: the end-to-end
  suite's report assertions and the report-document byte-pin test.
- **The same seam, one derivation case:** a daemon driven with a different
  build name serves `<other>.<code>` reasons - pinning ADR 0017's
  program-name derivation behaviorally. Prior art: the rules suite's existing
  canary build identity.
- **The `bandage -t` plan seam:** the plan prints the namespaced reason; the
  loader's validation at the same seam keeps its convention of a failing and a
  passing case - bare codes pass, dotted codes are refused. Prior art: the
  shipped-rules plan assertions and the rules suite.
- **The command's testable run seam:** one guard test asserting the program
  name is slug-shaped, per the ADR 0017 constraint (the constant is visible
  only here). This is the first test on this seam; the seam itself is
  pre-designed in the command.
- Existing domain, evaluation and persistence suites are updated only where
  they assert reason strings - expectation fallout, not feature specification.

## Out of Scope

- Any change to the consumer binary's compilation or an ingest-time
  validation of the reason pattern there (triaged as wontfix: the field is
  read-through only).
- Hand-maintained site configuration under the manually managed directory -
  untouched, including its reason codes.
- Legacy-row migration or read-path normalization: there is no production
  data to carry forward.
- Reason-based querying, matching, or filtering anywhere in the system.
- Per-rule or per-escalation prefixes: the prefix is one fixed namespace per
  producer.
- Namespacing rule IDs: they remain internal provenance, not part of the
  downstream contract (ADR 0004).
- Renaming the binary or the program name constant itself.

## Further Notes

- The split between ADR 0016 (the contract: any conforming prefix is legal)
  and ADR 0017 (the producer's choice: the program name) is load-bearing;
  consumers must never depend on the literal first label, and a future rename
  exercises exactly that boundary.
- The ADRs (0016, 0017, and the ADR 0004 superseded-in-part note) and the
  glossary's Reason entry have already landed in the repo; this spec is the
  implementation contract on top of them.
- The issue drafts previously sketched under this feature's `issues/`
  directory predate the design interview and are superseded by this spec
  (notably: no read-path normalization, a generic two-label wire pattern, and
  no consumer-side validation). Regenerate tickets from this spec.
