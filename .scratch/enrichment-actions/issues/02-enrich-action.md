# 02: Enrich action - demand-driven enrichment

**What to build:** A rules author writes an `enrich` action in a rule's
action list - `actions: [{enrich: {type: filesize}}]`, one enrichment
per step, order explicit in the list - and the named enrichment runs
where the rule matches: after that rule's tracker update, feeding the
derived fields of every subsequent rule. The documented composition -
a tracker-less enrich rule ahead of the consuming replay rule - turns
blind replay warm-up into live volume tracking. The enrichments are
global and static: one instance per kind, shared by every enrich action
naming it. The old world is gone in the same change: the pre-evaluation
enrichment path and the top-level `enrichment` block disappear, and an
old-shape rules file fails load with a pointed error naming the
migration. An unknown `type` or an unknown key in the enrich block
fails load naming it. The `bandage -t` plan shows enrich actions inline
in their rule's action list and prints no enrichment filter line.

**Blocked by:** None

**Status:** ready-for-agent

- [ ] An enrich action runs its named enrichment where every action runs (on Match, after its own rule's tracker update) and is non-terminal; derived fields feed subsequent rules only
- [ ] The tracker-less-enrich-rule-ahead-of-a-replay-rule composition works end to end at the assembled-daemon seam: reports appear where the tracker was blind before
- [ ] Demand-driven learning holds: an eligible response contributes an observation only when its event reaches an enrich action; untouched paths create no enrichment state
- [ ] One event traversing two enrich actions runs and observes twice - counted and documented as the rules author's responsibility, with no memoization
- [ ] The rules file's top-level `enrichment` block fails load with a pointed migration error; unknown `type` and unknown keys in the enrich block fail load naming them; an absent enrich block means no enrichment runs
- [ ] The plan prints enrich actions in their action list and no enrichment filter line
- [ ] Rules documentation covers the action, the ordering caveat and the tracker-less composition; the behavior is pinned by end-to-end tests, not internals
- [ ] The full suite passes race-enabled with the pre-evaluation path removed
