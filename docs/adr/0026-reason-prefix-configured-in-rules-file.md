# The reason prefix comes from the rules file

ADR 0016 leaves the reason-namespace prefix to the producer; ADR 0017
fixed bandage's choice at its program name. Deployments disagree: a
rebrand that outpaces a binary rename, two bandage instances feeding
one downstream consumer, or an operator who wants the namespace to name
the protected service rather than the executable - all want a prefix
that reflects the deployment, not the program. The prefix is therefore
configurable: the rules file may carry one top-level `reason_prefix`
key, slug-shaped like the authored code, and every report its rules
produce carries `<prefix>.<code>`. When the key is absent, the prefix
is the program name exactly as ADR 0017 had it - existing rules files
change nothing.

This supersedes ADR 0017 and revisits ADR 0016's rejected alternative
("making the prefix a rule-author concern, written in the rules file").
What 0016 rejected was per-rule prefixes fragmenting one producer's
namespace into unattributable shards; a single file-wide key cannot
fragment anything - one daemon, one namespace - and the rules author
and the operator are the same hand in practice. The report document
contract is untouched: 0016's shape stands, and only the producer's
choice of label becomes configurable.

Consequences: `progName` remains the build identity's name - version
stamps and `bandage_build_info` are unaffected, only reasons move; the
prefix shape is now load-time rules validation (a bad `reason_prefix`
fails `bandage -t`, never a runtime surprise), while the progname-shape
pin at the prefix-composition seam stays as the fallback's guard;
report-document examples pin `bandage.` as the default value only.
