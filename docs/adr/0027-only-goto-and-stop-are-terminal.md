# Only goto and stop are terminal

A report is a write, not a control-flow act. As part of ADR 0013's
terminality set (report, goto, stop) a report rung ended chain
traversal, so one flagged target swallowed every downstream rule for
that event: later rules' reasons - exactly the accumulating evidence
across rules that justifies a ban (ADR 0004) - never accrued, silently
and as a side effect of which rules carried report actions. The
terminal set shrinks to `goto` and `stop`: after a report action the
event continues through the chain's remaining rules, and a rule author
who wants traversal to end says so with an explicit `stop`. `goto`
keeps its meaning ("runs the target chain, then this chain ends"), and
`jump` was never terminal.

Considered: keeping report terminal and adding a per-action "continue"
modifier; rejected - it makes the safe behaviour opt-in and grows the
action vocabulary for what is a default fix. Amends ADR 0013's premise
that "terminal actions end chain traversal before later rules see the
event"; that was one motivation among several for escalation ladders,
which stand on staged responses alone and are unaffected.

Consequences: rules files that relied on a report ending the chain
must add `stop` to the report's rung to keep their old behaviour - the
shipped example configs are audited and annotated where a chain's
later rules now run; docs/rules.md's terminality statements follow the
new set; evaluation tests pin report-then-continue beside the existing
goto/stop terminality cases.
