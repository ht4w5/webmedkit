# 03: ADR 0032 - Filesize is served, not learned

**What to build:** The decision record for the whole change, per the
spec at `.scratch/filesize-served-not-learned/spec.md`: one new ADR,
"Filesize is served, not learned", covering the semantic flip (served
size at parse time; learning, EWMA and the Sampling rule retired) and
the wire-key removal. Following the house convention, the new ADR
stands alone and the older ADRs carry trailing notes: the two learning
ADRs marked superseded, the two whose rationale bends marked amended.
The rationale records the 206 decision - partial lengths must never
masquerade as file sizes, so a chunked download counts its true
fraction - the retained validation asymmetry (now justified as typed
field vs free-form header), and the reserved enrichment plumbing with
its empty kind vocabulary.

**Blocked by:** 01 - Served-size Filesize from the parser, and
02 - The sample loses its response-header fields (it records both)

**Status:** ready-for-agent

- [ ] One new ADR, "Filesize is served, not learned", covers the
      semantic flip and the wire-key removal
- [ ] The two learning ADRs carry "Superseded by" trailing notes; the
      two bent ADRs carry "Amended by" trailing notes (new ADR stands
      alone, older ones carry the note)
- [ ] The rationale records the 206 decision and its chunked-download
      scenario, the validation-asymmetry re-justification, and the
      reserved enrichment plumbing with an empty kind vocabulary
- [ ] Considered-and-rejected options are recorded (per-event partial
      length on 206; keeping the learned estimate; removing the
      enrichment plumbing wholesale)

## Comments
