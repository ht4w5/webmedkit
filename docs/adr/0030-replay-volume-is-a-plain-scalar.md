# Replay volume is a plain scalar

The replay volume methods (`constant`, `clamped-linear`,
`inverse-square`, `step`) computed a rule's tolerated volume as a
function of the estimated file size, and evaluation re-derived the
reading's volume - and the tracker cache-entry TTL - from the method
output per event. The indirection bought size-dependent tolerance that
no shipped rules file used, and it left a tracker's volume - the line
every overflow and lifecycle decision reads - varying per event. The
methods and their computation seam are gone: replay's `volume` is a
plain scalar in download equivalents and the reading's volume is
constant per rule. A volume-method block anywhere in a rules file
fails load through the ordinary quantity validation (a mapping is not
a quantity) - no dedicated check is kept for removed syntax - a
breaking rules-format change, accepted pre-release. The ladder `at`
quantity language is unchanged.

Considered: keeping the blocks with a deprecation path (warn and fall
back to the scalar), and keeping `constant` as a block spelling for
compatibility. Rejected: the format is pre-release, so a
`bandage -t` load failure migrates every rules file in one step, and a
half-supported method block is worse than its absence - the plain
scalar already is the constant method.

Consequences: the per-event volume re-derivation in evaluation is
replaced by the rule's constant; the tracker cache-entry TTL still
derives per event from it, and the `bandage -t` plan prints the
scalar. A volume constant per rule is what makes a dynamic report's
death line well-defined (one number for the whole report lifetime).
Rules files carrying a volume-method block migrate to a one-line
scalar in the tracker's quantity.
