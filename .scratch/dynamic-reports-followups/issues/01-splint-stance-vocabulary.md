# 01: splint stance wording follows the glossary

**What to build:** splint's report compiler never "demotes" a stance:
the glossary reserves **Downgrade** (and avoids "demotion") for a
dynamic report restating itself down its ladder, and splint's merge
logic is not that act - it is precedence resolution in compilation.
The compile comment and the cycle test name say "lower"; nothing else
moves.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] No "demote" wording remains in splint; the compile comment and
      the cycle test name say "lower"
- [x] No behavior change; the suite passes unchanged beyond the
      renames

## Comments

Implemented on integration branch `integration/dynamic-reports`
(commit `6a3875f`). The splint usage is compilation precedence -
`firewall.Stance` resolution in the merge - not the glossary's
Downgrade act, so "lower" is the plain word; "Downgrade" there would
conflate concepts.
Context: spec `../spec.md`.
