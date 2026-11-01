# Docs-consistency fix pass

**Origin:** project-wide consistency review of `docs/`, `GLOSSARY.md`,
and `README.md` against the code, followed by a grilling session that
settled every finding. This spec records the settled resolutions; the
tickets under `issues/` are the fix work.

## Decisions from the grill

- **Glossary binding scope**: `_Avoid_` lists bind docs and user-visible
  strings strictly. Code comments, test names, and identifiers are
  judgement calls, renamed only opportunistically. Already applied to
  `GLOSSARY.md`: "leaky bucket" names the mechanism (bare "bucket" never
  names a tracker); new term **Tracker record** (unqualified "record"
  means tracker state, never a Report); "a rung fires, a rule matches";
  dispatched chains run as subroutines, the entry chain never does.
- **Overflow vocabulary (inverted fix)**: "exceeds" is canonical; the
  glossary changed, `docs/reports.md:54` ("the overflow threshold
  `value` exceeded") stays. "Stands above" is swept everywhere in favour
  of "exceeds" (state) with "crosses" kept for the event; already
  applied to `GLOSSARY.md`.
- **splint malformed-report behaviour**: the doc changes, not the code.
  Loud failure on a structurally broken document is deliberate.
- **`track: ""` in tracker-less reports**: document the empty case; the
  wire contract stays byte-compatible (evolution policy).
- **Suppressions** (judgement calls, do not change): `firewall.Hit` /
  `Outcome.Hits`, `Stats()` / `track.Statistics`, `--nft-hook`, test
  names like `TestE2EEvictedBucketAccumulatesFresh`.
- **No ADRs**: glossary rescopes are cheaply reversible; Q6/Q7 confirm
  existing behaviour rather than decide it.

## Tickets

| # | Ticket | Sources |
|---|---|---|
| 01 | reports.md vocabulary + `track` enum | Q10, Q12, Q13, Q7 |
| 02 | `skipped report record` log line | Q11 |
| 03 | splint.md doc fixes | Q6, Q8-ii, Q14 |
| 04 | metrics.md snapshot wording | Q8-i |
| 05 | README operator list + ADR note + configs | Q8-iii |
| 06 | agent docs drift | Q8-iv, Q8-v |
| 07 | rules.md vocabulary sweep | Q15, Q17 |
| 08 | code comments | Q12, Q16 |
