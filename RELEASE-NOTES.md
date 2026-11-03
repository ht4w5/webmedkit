# Release notes

## Unreleased

### Breaking: report actions no longer end chain traversal

A `report` action writes its report and lets the event continue
through the chain's remaining rules - it no longer ends chain
traversal. Rules files that relied on a report ending the chain must
add `stop` beside the report to keep their old traversal. `goto` and
`stop` remain the terminal actions (`goto` runs the target chain, then
this chain ends; `jump` stays non-terminal) -
[ADR 0027](docs/adr/0027-only-goto-and-stop-are-terminal.md).

### New: configurable reason prefix

A top-level `reason_prefix` key names the namespace every report of
the file carries (`<prefix>.<code>`); an absent key keeps the
program-name prefix, so existing files change nothing -
[docs/rules.md](docs/rules.md),
[ADR 0026](docs/adr/0026-reason-prefix-configured-in-rules-file.md).

### New: rule counters

A rule with `counter: true` gets a Rule counter: the events it is
evaluated against and the subset that matched it, cumulative since
daemon start, surfaced in the `/api/stats` snapshot's `rules` block,
the Web Panel's Stats page and the `/metrics` rule families -
[docs/rules.md](docs/rules.md).
