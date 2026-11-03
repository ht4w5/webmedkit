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

**Status:** ready-for-agent

- [ ] A top-level `reason_prefix` key is accepted by the rules format,
  strictly decoded, and slug-validated at load with a key-naming error.
- [ ] Absent key: reports carry the program-name prefix exactly as
  today (existing namespacing behaviour passes unchanged).
- [ ] Present key: reports served by the report API carry the
  configured prefix; the authored code remains the reason's second
  label; build identity and version stamps are unaffected.
- [ ] `bandage -t` rejects an invalid prefix at load and prints the
  effective reason prefix in the plan.
- [ ] Rules-format documentation covers the key; the decision is
  recorded as an ADR superseding the program-name-prefix decision.
- [ ] Tests pin the above at the daemon end-to-end seam (served report
  reasons) and the rules-loader seam (load errors, defaults).

## Comments
