# 05: README operator list, ADR note, configs enumeration

**What to build:** `README.md` only. (a) Add `docs/metrics.md` to the
"Operator documentation" list. (b) Where ADR 0008 is cited (~:28), note
that ADR 0013 (in-place escalation) revises its tracker-reset
behaviour. (c) Complete the `configs/` enumeration: also
`nginx/bandage_log.conf`, `site/bandage.yml`, `site/rules.yml`.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Comments

Resolved in `README.md` (Q8-iii): `docs/metrics.md` added to the
operator list (after `ingest.md`), the ADR 0008 cite now notes that ADR
0013 revises its tracker-reset behavior, and the `configs/` enumeration
names `nginx/bandage_log.conf` and the git-ignored `site/` copies
(`site/bandage.yml`, `site/rules.yml`). Context: spec `../spec.md` Q8-iii,
`docs/metrics.md`, ADR
`docs/adr/0013-escalation-ladders-in-place-escalation.md`,
`.gitignore` `/configs/site`.
