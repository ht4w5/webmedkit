/* derived from google/pprof, Copyright Google Inc., Apache License 2.0 */
"use strict";

// Web Panel: the replicated pprof dropdown menus plus the two views'
// manual reloads - the Stats snapshot and the report review table.
// Data comes from the machine API only; nothing here polls - the
// REFRESH menu item is the only reload trigger.

// Menu dropdowns open on their menu-name click and retract on any
// outside press - pprof's menu behavior, kept minimal.
(function initMenus() {
	let openMenu = null;
	for (const item of document.querySelectorAll(".menu-item")) {
		const name = item.querySelector(".menu-name");
		const menu = item.querySelector(".submenu");
		if (!name || !menu) continue;
		name.addEventListener("click", (event) => {
			event.stopPropagation();
			const opening = openMenu !== menu;
			closeMenus();
			if (opening) {
				menu.style.display = "block";
				openMenu = menu;
			}
		});
	}
	function closeMenus() {
		if (openMenu === null) return;
		openMenu.style.display = "none";
		openMenu = null;
	}
	document.addEventListener("mousedown", (event) => {
		if (openMenu !== null && !event.target.closest(".menu-item")) {
			closeMenus();
		}
	});
})();

// fetchJSON fetches one machine API document and rejects on a non-OK
// response, so each loader's catch renders "error: ..." in its own slot.
function fetchJSON(method, url) {
	return fetch(url, { method }).then((res) => {
		if (!res.ok) {
			throw new Error(method + " " + url.split("?")[0] + " returned HTTP " + res.status);
		}
		return res.json();
	});
}

// The Stats view renders the snapshot as grouped key/value tables -
// identity, pipeline, tracker cache - one row per key the daemon
// reports, plus one counter table per counter block (the rule
// counters table now; a later counter table adopts the same
// mechanism). Rows are generated per reported key, never a fixed
// field list, so keys added to the snapshot later (ADR 0006: fields
// may be added) appear as generic extra rows and nothing the daemon
// reports is dropped by the panel.

// RULE_COUNTERS_KEY is the snapshot block feeding the rule counters
// table; its entries are counter rows, never key/value rows.
const RULE_COUNTERS_KEY = "rules";

// RULE_COUNTERS_SLOT is the rule counters table's row slot.
const RULE_COUNTERS_SLOT = "group-rule-counters";

// ENRICHMENT_COUNTERS_KEY is the snapshot block feeding the
// enrichment counters table; its entries are counter rows, never
// key/value rows.
const ENRICHMENT_COUNTERS_KEY = "enrichment";

// ENRICHMENT_COUNTERS_SLOT is the enrichment counters table's row slot.
const ENRICHMENT_COUNTERS_SLOT = "group-enrichment-counters";

// STATS_GROUPS maps the snapshot's known top-level blocks onto their
// group tables' row slots. The one map serves both the group dispatch
// and the row keys: a block the map names renders plain field keys -
// its group names the block - while any other block's generic extra
// rows keep the block-key prefix. A counter block's entry is the
// map's one documented shape exception: its entries are counter rows
// (the rule counters' are rule, total, matched; the enrichment
// counters' rule, enrichment, total, observed, enriched; GLOSSARY:
// Rule counter, Enrichment counter), which key/value rows cannot
// render (an array block has no
// field keys), so the dispatch below routes its value to the counter
// table's own row builder - still through this map and the
// counter-table registry, never a hard-coded key check.
const STATS_GROUPS = new Map([
	["build", "group-identity"],
	["pipeline", "group-pipeline"],
	["tracker_cache", "group-tracker-cache"],
	[RULE_COUNTERS_KEY, RULE_COUNTERS_SLOT],
	[ENRICHMENT_COUNTERS_KEY, ENRICHMENT_COUNTERS_SLOT],
]);

// GENERIC_GROUP_PREFIX marks a dynamically created group's row slot;
// the rest of the slot name is the block key its table is named for.
const GENERIC_GROUP_PREFIX = "group-extra-";

// genericGroups holds each generic group's created heading and table,
// so a render can clear them before rebuilding.
const genericGroups = new Map();

// valueText renders one reported value as row text.
function valueText(value) {
	return typeof value === "object" && value !== null ? JSON.stringify(value) : String(value);
}

// statsTables maps the snapshot document onto the groups - the one
// dispatch site: every top-level block routes through STATS_GROUPS.
// The pipeline and tracker_cache blocks feed their own tables field by
// field; the build block's fields and every other top-level scalar -
// generated_at, uptime, and any key added later - become identity rows.
// An unknown top-level block renders within its own generic group - a
// generic extra group table named for the block - never flattened into
// the identity table or hidden. A counter block (the map's documented
// shape exception) routes its entries to its counter table's own row
// builder instead of key/value rows.
function statsTables(snapshot) {
	const groups = { "group-identity": [], "group-pipeline": [], "group-tracker-cache": [] };
	// Every registered counter table starts with the empty set.
	const counterRows = new Map();
	for (const slot of COUNTER_TABLES.keys()) counterRows.set(slot, []);
	for (const [key, value] of Object.entries(snapshot)) {
		const slot = STATS_GROUPS.get(key);
		if (COUNTER_TABLES.has(slot)) {
			// The map's documented shape exception: counter rows,
			// never generic key/value rows.
			if (Array.isArray(value)) counterRows.set(slot, value);
			continue;
		}
		const block = typeof value === "object" && value !== null && !Array.isArray(value);
		const target = slot ??
			(block ? GENERIC_GROUP_PREFIX + key : "group-identity");
		const noKeyPrefix = STATS_GROUPS.has(key);
		const rows = block
			? Object.entries(value).map(([field, v]) => ({
				key: noKeyPrefix ? field : key + "." + field,
				value: valueText(v),
			}))
			: [{ key: key, value: valueText(value) }];
		(groups[target] = groups[target] || []).push(...rows);
	}
	return { groups: groups, counterRows: counterRows };
}

// kvRow builds one key/value row.
function kvRow(row) {
	const tr = document.createElement("tr");
	tr.className = "kv-row";
	for (const [className, text] of [["kv-key", row.key], ["kv-value", row.value]]) {
		const td = document.createElement("td");
		td.className = className;
		td.textContent = text;
		tr.appendChild(td);
	}
	return tr;
}

// KV_COLUMNS is the key/value tables' Key and Value columns.
const KV_COLUMNS = 2;

// messageRow builds one full-width message row (loading, no-match,
// error) spanning a table's columns, so a table is never blank.
function messageRow(text, colSpan) {
	const td = document.createElement("td");
	td.colSpan = colSpan;
	td.className = "empty";
	td.textContent = text;
	const tr = document.createElement("tr");
	tr.appendChild(td);
	return tr;
}

// renderStatsGroups renders each group's rows through the same row
// builder - the full fetched set or the search's narrowed view - so
// narrowing composes with everything the tables show. Generic group
// tables are rebuilt on every render, so a block the snapshot stops
// reporting leaves nothing stale behind.
function renderStatsGroups(groups, match) {
	for (const nodes of genericGroups.values()) {
		for (const node of nodes) node.remove();
	}
	genericGroups.clear();
	for (const [slot, rows] of Object.entries(groups)) {
		const tbody = groupBody(slot);
		if (tbody === null) continue;
		tbody.textContent = "";
		const shown = match === null ? rows : rows.filter((row) => match.test(row.key + "\n" + row.value));
		if (shown.length === 0) {
			tbody.appendChild(messageRow(match === null ? "Loading..." : "No keys match the filter.", KV_COLUMNS));
			continue;
		}
		for (const row of shown) {
			tbody.appendChild(kvRow(row));
		}
	}
}

// groupBody returns the group's row slot - the static page's own for
// the named groups, or for a generic group its table created here: a
// key/value table headed for its block.
function groupBody(slot) {
	const existing = document.getElementById(slot);
	if (existing !== null) return existing;
	if (!slot.startsWith(GENERIC_GROUP_PREFIX)) return null;
	const heading = document.createElement("h2");
	heading.className = "group-title";
	heading.textContent = slot.slice(GENERIC_GROUP_PREFIX.length);
	const table = document.createElement("table");
	table.className = "kv-table";
	const headRow = document.createElement("tr");
	for (const text of ["Key", "Value"]) {
		const th = document.createElement("th");
		th.textContent = text;
		headRow.appendChild(th);
	}
	const thead = document.createElement("thead");
	thead.appendChild(headRow);
	const tbody = document.createElement("tbody");
	tbody.id = slot;
	table.appendChild(thead);
	table.appendChild(tbody);
	const content = document.getElementById("content");
	content.appendChild(heading);
	content.appendChild(table);
	genericGroups.set(slot, [heading, table]);
	return tbody;
}

// The Stats page's counter tables - the rule counters table now, and
// any later counter table - render through one counter-table
// mechanism: one renderer plus one config per table. The config names
// the table's row slot, its row builder, its search fields, its
// explicit empty-state copy and its shape - whose one source is the
// served header row the config derives its column count from - so a
// second counter table adopts the mechanism instead of duplicating
// the machinery.

// ruleCountersColumns takes the rule counters table's column count
// from its own header row - the served markup is the table's one shape
// source - so the full-width message rows and the page's columns
// cannot drift.
function ruleCountersColumns() {
	return document.getElementById(RULE_COUNTERS_SLOT).closest("table").querySelectorAll("thead th").length;
}

// ruleCounterRow builds one rule counter row: rule, total, matched.
function ruleCounterRow(counter) {
	const tr = document.createElement("tr");
	tr.className = "rule-counter-row";
	for (const text of [counter.id, String(counter.total), String(counter.matched)]) {
		const td = document.createElement("td");
		td.textContent = text;
		tr.appendChild(td);
	}
	return tr;
}

// RULE_COUNTERS_TABLE is the rule counters table's counter-table
// config: its row slot, its row builder (rule, total, matched;
// GLOSSARY: Rule counter), its search fields - rule, total and
// matched - its explicit empty-state copy and its header-derived
// shape.
const RULE_COUNTERS_TABLE = {
	slot: RULE_COUNTERS_SLOT,
	row: ruleCounterRow,
	narrow: (counter, match) => match.test(counter.id + "\n" + counter.total + "\n" + counter.matched),
	empty: "No rule counters configured.",
	columns: ruleCountersColumns,
};

// COUNTER_TABLES registers every counter table's config by its row
// slot, so statsTables routes a counter block to its table - still
// through STATS_GROUPS, never a hard-coded key check.
const COUNTER_TABLES = new Map([
	[RULE_COUNTERS_SLOT, RULE_COUNTERS_TABLE],
	[ENRICHMENT_COUNTERS_SLOT, ENRICHMENT_COUNTERS_TABLE],
]);

// renderCounterTable renders one counter table - the full fetched set
// or the search's narrowed view, through the table's own row builder
// and search fields - with the table's explicit empty state when no
// counter is configured, so silence is never mistaken for a broken
// panel.
function renderCounterTable(table, counters, match) {
	const tbody = document.getElementById(table.slot);
	if (tbody === null) return;
	tbody.textContent = "";
	const all = counters === null ? [] : counters;
	const shown = match === null ? all : all.filter((counter) => table.narrow(counter, match));
	if (shown.length === 0) {
		let copy;
		if (counters === null) copy = "Loading...";
		else if (match !== null) copy = "No keys match the filter.";
		else copy = table.empty;
		tbody.appendChild(messageRow(copy, table.columns()));
		return;
	}
	for (const counter of shown) {
		tbody.appendChild(table.row(counter));
	}
}

// renderRuleCounters renders the rule counters table through the
// shared counter-table mechanism.
function renderRuleCounters(counters, match) {
	renderCounterTable(RULE_COUNTERS_TABLE, counters, match);
}

// enrichmentCountersColumns takes the enrichment counters table's
// column count from its own header row - the served markup is the
// table's one shape source - so the full-width message rows and the
// page's columns cannot drift.
function enrichmentCountersColumns() {
	return document.getElementById(ENRICHMENT_COUNTERS_SLOT).closest("table").querySelectorAll("thead th").length;
}

// enrichmentCounterRow builds one enrichment counter row: rule,
// enrichment, total, observed, enriched.
function enrichmentCounterRow(counter) {
	const tr = document.createElement("tr");
	tr.className = "enrichment-counter-row";
	for (const text of [counter.rule, counter.enrichment, String(counter.total), String(counter.observed), String(counter.enriched)]) {
		const td = document.createElement("td");
		td.textContent = text;
		tr.appendChild(td);
	}
	return tr;
}

// ENRICHMENT_COUNTERS_TABLE is the enrichment counters table's
// counter-table config: its row slot, its row builder (rule,
// enrichment, total, observed, enriched; GLOSSARY: Enrichment
// counter), its search fields - rule, enrichment, total, observed and
// enriched - its explicit empty-state copy and its header-derived
// shape.
const ENRICHMENT_COUNTERS_TABLE = {
	slot: ENRICHMENT_COUNTERS_SLOT,
	row: enrichmentCounterRow,
	narrow: (counter, match) => match.test(counter.rule + "\n" + counter.enrichment + "\n" + counter.total + "\n" + counter.observed + "\n" + counter.enriched),
	empty: "No enrichment counters configured.",
	columns: enrichmentCountersColumns,
};

// renderEnrichmentCounters renders the enrichment counters table
// through the shared counter-table mechanism.
function renderEnrichmentCounters(counters, match) {
	renderCounterTable(ENRICHMENT_COUNTERS_TABLE, counters, match);
}

// fetchedStatsGroups holds the three groups' rows from the last fetch;
// anything narrowing the view only ever re-renders a filtered view.
let fetchedStatsGroups = {};

// fetchedRuleCounters holds the rules block from the last fetch - the
// rule counters table's rows; null before the first fetch (the loading
// row), an empty list after one reporting no counters (the explicit
// empty state).
let fetchedRuleCounters = null;

// fetchedEnrichmentCounters holds the enrichment block from the last
// fetch - the enrichment counters table's rows; null before the first
// fetch (the loading row), an empty list after one reporting no
// counters (the explicit empty state).
let fetchedEnrichmentCounters = null;

// fillLegend puts the running binary's identity and uptime in the
// header's right-aligned legend - build identity and uptime at a
// glance (the identity table carries the full snapshot).
function fillLegend(snapshot) {
	const legend = document.getElementById("legend");
	if (legend === null) return;
	const build = snapshot.build || {};
	const identity = [build.name, build.version].filter(Boolean).join(" ");
	legend.textContent = (build.commit ? identity + " (" + build.commit + ")" : identity) +
		" · uptime " + snapshot.uptime;
}

// loadStats renders the Stats snapshot - the human-facing document with
// no format contract (ADR 0006) - as the page's grouped key/value
// tables.
function loadStats() {
	if (document.getElementById("group-identity") === null) return;
	fetchJSON("GET", "/api/stats")
		.then((snapshot) => {
			const tables = statsTables(snapshot);
			fetchedStatsGroups = tables.groups;
			fetchedRuleCounters = tables.counterRows.get(RULE_COUNTERS_SLOT);
			fetchedEnrichmentCounters = tables.counterRows.get(ENRICHMENT_COUNTERS_SLOT);
			fillLegend(snapshot);
			applyStatsFilter();
		})
		.catch((err) => {
			for (const tbody of document.querySelectorAll("#content tbody")) {
				tbody.textContent = "";
				const columns = tbody.closest("table").querySelectorAll("thead th").length;
				tbody.appendChild(messageRow("error: " + err.message, columns));
			}
		});
}

// Report review table: one row per live Report fetched from
// /api/reports; reportRow(report) builds one row's markup including its
// delete cell.

// The review table's columns: rule, target, tracker reading, updated
// and the delete cell - detail and message rows span all of them.
const REPORT_COLUMNS = 5;

function trackerLabel(tracker) {
	if (!tracker || !tracker.track) {
		return "none";
	}
	return tracker.track + " " + tracker.value + "/" + tracker.volume;
}

function reportRow(report) {
	const row = document.createElement("tr");
	row.className = "report-row";
	row.setAttribute("aria-expanded", "false");
	for (const text of [report.rule, report.target, trackerLabel(report.tracker), report.updated_at]) {
		const td = document.createElement("td");
		td.textContent = text;
		row.appendChild(td);
	}
	// The delete button lives in its own cell; its click handler stops
	// propagation so it never toggles the row.
	row.appendChild(deleteCell(report));
	const detail = document.createElement("tr");
	detail.className = "report-detail";
	detail.hidden = true;
	const td = document.createElement("td");
	td.colSpan = REPORT_COLUMNS;
	td.className = "report-detail-cell";
	const pre = document.createElement("pre");
	pre.className = "json";
	// The full report body pretty-printed: reason, penalty, sample,
	// expiry and the rest of the entry.
	pre.textContent = JSON.stringify(report, null, 2);
	td.appendChild(pre);
	detail.appendChild(td);
	// The toggle reaches only this row's own detail, so expanding one
	// row never disturbs the others. (The delete button stops click
	// propagation so it does not toggle the row.)
	row.addEventListener("click", () => {
		const open = detail.hidden;
		detail.hidden = !open;
		row.setAttribute("aria-expanded", open ? "true" : "false");
	});
	const pair = document.createDocumentFragment();
	pair.appendChild(row);
	pair.appendChild(detail);
	return pair;
}

// showEmptyState swaps the review table and the empty state in and out
// together: an empty live set reads as information - "No live reports."
// - instead of a bare table.
function showEmptyState(show) {
	document.getElementById("report-table").hidden = show;
	const empty = document.getElementById("empty-state");
	empty.hidden = !show;
	empty.textContent = show ? "No live reports." : "";
}

// renderReports renders whatever list it is handed - the full fetched
// one or the filter's narrowed view - swapping in the empty state when
// nothing at all is live.
function renderReports(list) {
	const rows = document.getElementById("report-rows");
	rows.textContent = "";
	if (list.length === 0) {
		showEmptyState(true);
		return;
	}
	showEmptyState(false);
	for (const report of list) {
		rows.appendChild(reportRow(report));
	}
}

// showReportsError replaces the report rows with one error row, so a
// failed fetch never leaves the table blank.
function showReportsError(err) {
	const rows = document.getElementById("report-rows");
	showEmptyState(false);
	rows.textContent = "";
	rows.appendChild(messageRow("error: " + err.message, REPORT_COLUMNS));
}

function loadReports() {
	const rows = document.getElementById("report-rows");
	if (rows === null) return;
	showEmptyState(false);
	rows.textContent = "";
	rows.appendChild(messageRow("Loading...", REPORT_COLUMNS));
	fetchJSON("GET", "/api/reports")
		.then((doc) => {
			fetchedReports = doc.reports || [];
			// The header's legend shows the live report count - the
			// full fetched total, which the narrowed view never
			// changes.
			document.getElementById("legend").textContent =
				"Live reports: " + fetchedReports.length;
			applyReportFilter();
		})
		.catch(showReportsError);
}

// Per-row delete: its own cell, and the button stops click propagation
// so clicking it never toggles the row's expansion. Delete is two-step
// (click, then confirm): the first click arms the confirmation and only
// the confirm control issues the DELETE - by the report's rule and
// target - after which the list refetches so the deleted Report
// is gone. Cancelling puts the Delete button back and deletes nothing.
const DELETE_CONFIRM_COPY =
	"Deleting this report withdraws the enforcement derived from it at " +
	"the enforcing consumer's next sync.";

function armDeleteButton(td, report) {
	const button = document.createElement("button");
	button.type = "button";
	button.textContent = "Delete";
	button.addEventListener("click", (event) => {
		event.stopPropagation();
		showDeleteConfirm(td, report);
	});
	td.appendChild(button);
}

function deleteCell(report) {
	const td = document.createElement("td");
	td.className = "delete";
	armDeleteButton(td, report);
	return td;
}

function showDeleteConfirm(td, report) {
	td.textContent = "";
	const copy = document.createElement("p");
	copy.className = "delete-confirm";
	copy.textContent = DELETE_CONFIRM_COPY;
	td.appendChild(copy);
	const confirm = document.createElement("button");
	confirm.type = "button";
	confirm.textContent = "Confirm";
	confirm.addEventListener("click", (event) => {
		event.stopPropagation();
		deleteReport(report);
	});
	td.appendChild(confirm);
	const cancel = document.createElement("button");
	cancel.type = "button";
	cancel.textContent = "Cancel";
	cancel.addEventListener("click", (event) => {
		event.stopPropagation();
		td.textContent = "";
		armDeleteButton(td, report);
	});
	td.appendChild(cancel);
}

function deleteReport(report) {
	const url = "/api/reports?rule=" + encodeURIComponent(report.rule) +
		"&target=" + encodeURIComponent(report.target);
	fetchJSON("DELETE", url)
		.then(() => {
			loadReports();
		})
		.catch(showReportsError);
}

// fetchedReports holds the full list from the last fetch; anything
// narrowing the view only ever re-renders a filtered list of it.
let fetchedReports = [];

// Report filtering: the header's regexp search box narrows the fetched
// report list client-side - an unanchored, case-insensitive search over
// rule, target, and the stringified body, the same behaviour the
// panel's old report filter had. This is a panel convenience over
// stored Reports, deliberately independent of the Filter DSL's regex
// matchers, which evaluate Events during rule evaluation.

// searchMatcher compiles the header's search box for either view: null
// for the empty box (no narrowing), a RegExp otherwise. A typo never
// blanks the view - an invalid pattern is called out inline and the
// full rows stay visible until it is fixed.
function searchMatcher() {
	const pattern = document.getElementById("search").value;
	const error = document.getElementById("search-error");
	if (pattern === "") {
		error.hidden = true;
		error.textContent = "";
		return { match: null, invalid: false };
	}
	try {
		const match = new RegExp(pattern, "i");
		error.hidden = true;
		error.textContent = "";
		return { match: match, invalid: false };
	} catch (err) {
		error.textContent = "invalid pattern: " + err.message;
		error.hidden = false;
		return { match: null, invalid: true };
	}
}

function applyReportFilter() {
	const rows = document.getElementById("report-rows");
	if (rows === null) return;
	const { match, invalid } = searchMatcher();
	// An invalid pattern keeps the full list visible (the inline error
	// names the typo); clearing the box restores the full list too.
	if (invalid || match === null) {
		renderReports(fetchedReports);
		return;
	}
	// The narrowed list re-renders through renderReports - the same
	// row builder that carries the expand toggle - so filtering
	// composes with row expansion.
	const narrowed = fetchedReports.filter((report) =>
		match.test(report.rule + "\n" + report.target + "\n" + JSON.stringify(report)),
	);
	if (narrowed.length === 0 && fetchedReports.length > 0) {
		rows.textContent = "";
		rows.appendChild(messageRow("No reports match the filter.", REPORT_COLUMNS));
		return;
	}
	renderReports(narrowed);
}

// applyStatsFilter narrows the visible rows to the search box's
// matches - over key and value, unanchored and case-insensitive, and
// over the rule and enrichment counter rows the same way - so the
// search box behaves the same whichever view is on screen. An invalid
// pattern leaves the full rows visible.
function applyStatsFilter() {
	if (document.getElementById("group-identity") === null) return;
	const { match, invalid } = searchMatcher();
	const filter = invalid ? null : match;
	renderStatsGroups(fetchedStatsGroups, filter);
	renderRuleCounters(fetchedRuleCounters, filter);
	renderEnrichmentCounters(fetchedEnrichmentCounters, filter);
}

// applySearch runs the search box against whichever view's rows are on
// the page (each filter bails when its slots are absent).
function applySearch() {
	applyReportFilter();
	applyStatsFilter();
}

// refreshAll is the one manual reload trigger: both views' data refetch
// on REFRESH (each page renders its own slot); nothing here polls.
function refreshAll() {
	loadStats();
	loadReports();
}

document.getElementById("refresh").addEventListener("click", refreshAll);
document.getElementById("search").addEventListener("input", applySearch);

refreshAll();
