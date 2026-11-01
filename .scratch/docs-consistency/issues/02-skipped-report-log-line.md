# 02: `skipped report record` log line

**What to build:** Rename the user-visible log message in
`splint/internal/app/updater.go:239` from `Msg("skipped report record")`
to `Msg("skipped report")`. Update the two references in
`docs/splint.md` (:225 sample output, :679 prose) so the sample output
stays byte-accurate. Check the accompanying skip counter's name for
"record" and align if present. No behaviour change.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent
