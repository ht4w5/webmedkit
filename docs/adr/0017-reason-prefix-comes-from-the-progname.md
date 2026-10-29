# The reason prefix comes from the program name

ADR 0016 leaves each producer free to choose its reason-namespace
prefix. bandage's choice: the prefix is its program name plus a dot —
`progName`, the single constant in `bandage/cmd/bandage/main.go`,
reaching report creation through the build identity
(`identity.For(progName)` → `identity.Build.Name()`), the channel the
version stamps already ride. A program rename therefore re-namespaces
every reason automatically: the tool's names stay in step with one edit
— the motivation, since this project renamed itself webmedkit while the
binaries deliberately kept their names.

This is a producer-side decision, not part of the report document
contract: any conforming prefix is legal (ADR 0016), and a rename
changes reason values without breaking conformance or bumping the
document version.

Consequences: `progName` must stay slug-shaped
(`^[a-z0-9][a-z0-9_-]*$`) — pinned by a test at the prefix-composition
seam rather than by startup validation of a compile-time constant;
report-document examples pin `bandage.` as the current value only.
