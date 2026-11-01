# 02: `skipped report record` log line

**What to build:** Rename the user-visible log message in
`splint/internal/app/updater.go:239` from `Msg("skipped report record")`
to `Msg("skipped report")`. Update the two references in
`docs/splint.md` (:225 sample output, :679 prose) so the sample output
stays byte-accurate. Check the accompanying skip counter's name for
"record" and align if present. No behaviour change.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Comments

Resolved on branch `ticket/dc-02-skipped-report-log-line`. The log
message is now `Msg("skipped report")`; the `docs/splint.md` sample
output and prose echo it byte-accurately. The skip counter is named
`skipped` (no "record" to align) and the test pin in `cycle_test.go`
follows the rename.

Context: spec `../spec.md` (ticket 02, Q11; glossary: unqualified
"record" means tracker state, never a Report).
