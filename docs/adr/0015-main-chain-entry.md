# Only the main chain is evaluated per event

bandage used to run every chain over every event in file order (flux's
shape), which made every chain an entry point: declaration order silently
mattered, and chains meant as dispatch targets saw events they were
never dispatched to - a `goto`'d chain took the triggering event twice,
once on its own pass and once on the handoff. We decide instead that per
event evaluation enters exactly one chain, the entry chain named `main`;
`goto`/`jump` are the only way to enter any other chain. Chains
unreachable from `main` are legal and inert (like iptables custom
chains), and `goto`/`jump` cannot target `main` (in-degree 0; a reachable
attempt would be a cycle anyway).

Considered options: every-chain traversal (rejected: the bugs above);
configurable or multiple entry chains (rejected: no v0 use case, YAGNI);
deduplicating dispatch to once per chain per event (rejected: needs
per-event evaluator state - a chain is a subroutine, calling it twice
runs it twice; the pitfall is documented instead).

Consequences: a `goto`'d chain no longer runs its own top-level pass, so
its trackers take each handoff event once; the `--test` plan format is
unchanged.
