# 02: Report actions are non-terminal

**What to build:** A report action stops ending chain traversal: a
Trigger writes the report and the Event continues through the chain's
remaining rules, so reasons accrue across rules and one flag no longer
blinds everything written after it. Ending traversal becomes explicit -
a `stop` action, or `goto`'s existing "target chain, then this chain
ends" - exactly where the rules author wrote it. `jump` stays
non-terminal and escalation ladders are unchanged (every crossed rung
still fires lowest first). The shipped example rules are audited and
annotated where a chain's later rules now run.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A rung carrying a report action is not terminal: the event
  reaches the chain's remaining rules (pinned at the daemon
  end-to-end seam).
- [x] `stop` and `goto` remain terminal; `jump` remains non-terminal;
  ladder firing (decided once, lowest first, no rung cancelled) is
  unchanged.
- [x] Shipped example rules audited; behaviour changes annotated where
  a chain's later rules now evaluate reported events.
- [x] Rules-format documentation's terminality statements updated; the
  decision recorded as an ADR amending the escalation-ladder premise.
- [x] Tests pin report-then-continue beside the goto/stop terminality
  cases and the unchanged ladder traversal.

## Answer

Implemented on `ticket/ep-02-nonterminal-report` (commit `33a14d3`),
merged into `integration/eval-pipeline` (`e32a509`).
`Escalation.Terminal()` shrinks to goto/stop: a report action writes
and the Event continues through the chain's remaining rules; `jump`
stays non-terminal; ladder firing (decided once pre-action, lowest
first, no rung cancelled) unchanged. The three tests pinning report
terminality were rewritten to report-then-continue
(`TestE2EReportContinuesTheChainTraversal`,
`TestE2EEscalationReportRungContinuesTheChain`, replay-ladder marks
updated). Shipped `configs/rules.yml` audited: one behaviour change,
annotated at `iso-download-flood` (`download-abuse` chain) - a later
rule now evaluates Events `sustained-iso-egress` reported; `stop`
restores the old traversal. docs/rules.md terminality statements
follow ADR 0027. The release-notes callout landed in the review fix
pass (`0c52ba5`, merged `a9da2ad`).

Context: spec `../spec.md`, ADR `docs/adr/0027` (amending ADR 0013).

## Comments
