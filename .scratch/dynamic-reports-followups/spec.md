# Spec: dynamic-reports follow-ups - review leftovers

Status: ready-for-agent

## Problem Statement

The dynamic-reports change landed with three open items its code
review raised but scoped out: splint still uses the glossary's avoided
"demote" synonym for stance lowering, `report.Service.Delete` drops
live-set entries while a write for the pair is in flight (a waiter can
hang on the orphaned channel), and the dynamic-reports spec still
promises a pointed "error naming the removal" that the
no-checks-for-removed-syntax decision superseded.

## Solution

The vocabulary fix is wording only: splint's merge logic lowers a
stance in compilation, it never Downgrades a report, so it says
"lower". `Delete` keeps in-flight pairs in the live set - their writes
land after the delete and stand - matching `DeleteExpired` and
`DeleteDynamic`'s skip discipline; and the in-flight channel stays the
writer's to close, exactly once, on every settle and failure path, so
no waiter can ever hang. The dynamic-reports spec is amended to the
shipped refusal: the ordinary quantity validation fails a
volume-method block at `bandage -t`, with no dedicated check kept for
removed syntax.

## Out of Scope

- Anything the dynamic-reports tickets recorded as accepted (restart
  amnesty, margin wider than volume, the Sample kept on Downgrade).
