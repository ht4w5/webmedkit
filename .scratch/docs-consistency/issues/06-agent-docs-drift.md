# 06: agent docs drift

**What to build:** (a) `docs/agents/triage-labels.md`: remove the
template leftovers - the "Label in mattpocock/skills" column header and
the closing "Edit the 'Label in our tracker' column to match whatever
vocabulary you actually use." line - the five-role vocabulary is
settled per AGENTS.md. (b) `docs/agents/issue-tracker.md`: note that
the `.scratch/<feature-slug>/spec.md` convention applies to new feature
dirs (`.scratch/rename/` predates it and holds only `handoff.md`), or
state the exception explicitly.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Comments

Context: resolved on branch `ticket/dc-06-agent-docs-drift` (spec
`../spec.md`, Q8-iv/Q8-v). `docs/agents/triage-labels.md` now carries a
single label column - the `mattpocock/skills` mapping header and the
closing "edit the column" template line are gone (the five-role
vocabulary is settled per AGENTS.md). `docs/agents/issue-tracker.md`
scopes the `spec.md` convention to new feature dirs and states the
`.scratch/rename/` exception (predates it, holds only `handoff.md`).
`make test` green (docs-only; the doc-pinning suites still pass).
