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

**Status:** ready-for-agent

- [ ] A rung carrying a report action is not terminal: the event
  reaches the chain's remaining rules (pinned at the daemon
  end-to-end seam).
- [ ] `stop` and `goto` remain terminal; `jump` remains non-terminal;
  ladder firing (decided once, lowest first, no rung cancelled) is
  unchanged.
- [ ] Shipped example rules audited; behaviour changes annotated where
  a chain's later rules now evaluate reported events.
- [ ] Rules-format documentation's terminality statements updated; the
  decision recorded as an ADR amending the escalation-ladder premise.
- [ ] Tests pin report-then-continue beside the goto/stop terminality
  cases and the unchanged ladder traversal.

## Comments
