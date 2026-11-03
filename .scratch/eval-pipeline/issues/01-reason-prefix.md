# 01: Reason prefix configured in the rules file

**What to build:** A rules author writes a top-level `reason_prefix`
key in their rules file and every report the file's rules create
carries `<prefix>.<code>` instead of the program-name prefix - one
file, one namespace. An absent key keeps today's reasons byte-identical
(no migration); a present key must be slug-shaped or the rules file
fails load, `bandage -t` included, with an error naming the key. The
`--test` plan prints the effective reason prefix so the deployment's
namespace is visible before shipping. The report document contract is
untouched: the authored code stays the reason's second label and the
build identity keeps the program name.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A top-level `reason_prefix` key is accepted by the rules format,
  strictly decoded, and slug-validated at load with a key-naming error.
- [x] Absent key: reports carry the program-name prefix exactly as
  today (existing namespacing behaviour passes unchanged).
- [x] Present key: reports served by the report API carry the
  configured prefix; the authored code remains the reason's second
  label; build identity and version stamps are unaffected.
- [x] `bandage -t` rejects an invalid prefix at load and prints the
  effective reason prefix in the plan.
- [x] Rules-format documentation covers the key; the decision is
  recorded as an ADR superseding the program-name-prefix decision.
- [x] Tests pin the above at the daemon end-to-end seam (served report
  reasons) and the rules-loader seam (load errors, defaults).

## Answer

Implemented on `ticket/ep-01-reason-prefix` (commits `4b06896`,
`9e6133a`, `2646d0f`, `2c4802c`), merged into `integration/eval-pipeline`
(`ce70f00`). The rules format gains a top-level `reason_prefix`,
slug-validated at load with key-naming errors; the loader carries
`Rules.ReasonPrefix` and `Rules.effectiveReasonPrefix(build.Name())`
chooses the effective prefix at daemon assembly (`app/daemon.go`),
feeding the evaluator so report reasons namespace at creation exactly
as ADR 0026 requires. The `--test` plan prints `reason prefix:
<prefix>` and namespaces its report reasons; the absent key keeps the
byte-identical `bandage.*` fallback (existing pins untouched). ADR
0026 records the decision; ADR 0017 carries the supersession note.
Review fix pass (`0c2259c`, merged `a9da2ad`) tightened validation per
adjudication: a written key must carry a valid string slug - written
null/non-string scalars fail load naming the key; absent means the
program name. Pins: `TestParseRulesReasonPrefix`,
`TestPlanReasonPrefix`,
`TestE2EConfiguredReasonPrefixNamespacesServedReports`.

Context: spec `../spec.md` (Implementation/Testing Decisions), ADR
`docs/adr/0026`.

## Comments
