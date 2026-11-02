# 08: Client-side regex filter over the report list

**What to build:** The Reports tab gains one filter box that narrows the
fetched report list client-side: an unanchored, case-insensitive regular
expression matched across rule, Target, and stringified body. An invalid
pattern shows an inline error and leaves the list unfiltered until fixed.
This is a panel convenience for report review, deliberately independent of
the Filter DSL's regex matchers, which evaluate Events during rule
evaluation.

**Blocked by:** 06 - Reports tab: report review table with expandable bodies

**Status:** resolved

- [x] Typing a pattern filters the visible rows immediately across rule, Target, and body text.
- [x] Matching is unanchored and case-insensitive (a fragment finds its reports).
- [x] An invalid pattern shows an inline error and leaves the full list visible.
- [x] Clearing the box restores the full list.
- [x] Filtering composes with row expansion (expandable rows still work on the filtered list).

## Answer

Implemented on `ticket/wam-08-report-regex-filter` (commit
`0627dfd`), merged into `integration/web-panel-api-move` (`15012b4`).
One filter input narrows the fetched report list client-side:
`new RegExp(pattern, "i")` - unanchored, case-insensitive - matched
across rule, Target, and the stringified body. Typing filters
immediately; clearing restores the full list; an invalid pattern shows
an inline error and leaves the full list visible. Filtering composes
with row expansion (filtered subsets re-render through the same row
builder) and with ticket 07's delete/refetch flow (the refetch feeds
the filter's cached list). The panel comments state this filter
operates on stored Reports and is independent of the Filter DSL's
regex matchers. Reviewed in the fix pass `62e3efa`.
