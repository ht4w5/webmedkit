# 03: Docs read true

**What to build:** No documentation surface describes a warm-up gate
that no longer exists. The glossary's Filesize entry states that a
path's estimate exists from its first observation and each later
observation refines it, unknown until that first observation - with the
empty-content indistinguishability sentence kept and "encounter" not
defined anywhere. ADR 0019 is amended (its amendment note extended)
with its warm-up consequences reworded, including the rollout note:
after an emitter upgrade, one observation now suffices instead of
several.

**Blocked by:** 01, 02

**Status:** resolved

- [x] Glossary Filesize entry reworded per the spec; no "encounter"
      term introduced
- [x] ADR 0019 amended and its warm-up wording reworded to match
      shipped behaviour
- [x] No documentation describes the estimate as waiting for several
      observations

Implemented on integration branch `integration/filesize-first-encounter`
(merge commit `789fa2a`). Beyond the ticket's named surfaces, ADR 0002's
`minSamples` consequence and a `configs/rules.yml` sample comment were
reworded minimally to satisfy checklist item 3. Open at resolve time:
ADR 0019's amendment note follows the earlier in-place style rather
than the repo's numbered amendment-ADR pattern (see the code review;
human decision pending).
Context: spec `../spec.md` (Implementation Decisions; Further Notes).

## Comments

The pending ADR amendment-style question resolved as option (b): the
warm-up-gate decision moved to ADR 0029 and ADR 0019 reads
`Amended by ADR 0029`, matching the repo's numbered amendment-ADR
pattern.
