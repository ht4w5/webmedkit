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
// reports. Rows are generated per reported key, never a fixed field
// list, so keys added to the snapshot later (ADR 0006: fields may be
// added) appear as generic extra rows and nothing the daemon reports
// is dropped by the panel.

// STATS_SLOTS names each group's row slot, in display order.
const STATS_SLOTS = ["group-identity", "group-pipeline", "group-tracker-cache"];

// valueText renders one reported value as row text.
function valueText(value) {
	return typeof value === "object" && value !== null ? JSON.stringify(value) : String(value);
}

// statsGroups maps the snapshot document onto the three groups: the
// pipeline and tracker_cache blocks feed their own tables field by
// field; the build block's identity fields and every other top-level
// entry - generated_at, uptime, and any key added later - become
// identity rows. A new top-level block flattens into dotted generic
// rows rather than being hidden.
function statsGroups(snapshot) {
	const groups = { "group-identity": [], "group-pipeline": [], "group-tracker-cache": [] };
	for (const [key, value] of Object.entries(snapshot)) {
		let rows;
		if (typeof value === "object" && value !== null && !Array.isArray(value)) {
			const bare = key === "build" || key === "pipeline" || key === "tracker_cache";
			rows = Object.entries(value).map(([field, v]) => ({
				key: bare ? field : key + "." + field,
				value: valueText(v),
			}));
		} else {
			rows = [{ key: key, value: valueText(value) }];
		}
		if (key === "pipeline") {
			groups["group-pipeline"].push(...rows);
		} else if (key === "tracker_cache") {
			groups["group-tracker-cache"].push(...rows);
		} else {
			groups["group-identity"].push(...rows);
		}
	}
	return groups;
}

// kvRow builds one key/value row; kvMessageRow the full-width message
// row (loading, error) the key/value tables share.
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

function kvMessageRow(text) {
	const td = document.createElement("td");
	td.colSpan = 2;
	td.className = "empty";
	td.textContent = text;
	const tr = document.createElement("tr");
	tr.appendChild(td);
	return tr;
}

// renderStatsGroups renders each group's rows through the same row
// builder - the full fetched set or the search's narrowed view - so
// narrowing composes with everything the tables show.
function renderStatsGroups(groups, match) {
	for (const [slot, rows] of Object.entries(groups)) {
		const tbody = document.getElementById(slot);
		if (tbody === null) continue;
		tbody.textContent = "";
		const shown = match === null ? rows : rows.filter((row) => match.test(row.key + "\n" + row.value));
		if (shown.length === 0) {
			tbody.appendChild(kvMessageRow(match === null ? "Loading..." : "No keys match the filter."));
			continue;
		}
		for (const row of shown) {
			tbody.appendChild(kvRow(row));
		}
	}
}

// fetchedStatsGroups holds the three groups' rows from the last fetch;
// anything narrowing the view only ever re-renders a filtered view.
let fetchedStatsGroups = {};

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
			fetchedStatsGroups = statsGroups(snapshot);
			fillLegend(snapshot);
			applyStatsFilter();
		})
		.catch((err) => {
			for (const slot of STATS_SLOTS) {
				const tbody = document.getElementById(slot);
				if (tbody === null) continue;
				tbody.textContent = "";
				tbody.appendChild(kvMessageRow("error: " + err.message));
			}
		});
}

// Report review table: one row per live Report fetched from
// /api/reports. renderReports(list) renders whatever list it is handed -
// the full fetched one or the filter's narrowed view - and
// reportRow(report) builds one row's markup including its delete cell.

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

// messageRow renders one full-width message row (loading, empty state,
// error) so the table is never blank.
function messageRow(text) {
	const td = document.createElement("td");
	td.colSpan = REPORT_COLUMNS;
	td.className = "empty";
	td.textContent = text;
	const tr = document.createElement("tr");
	tr.appendChild(td);
	return tr;
}

function renderReports(list) {
	const rows = document.getElementById("report-rows");
	rows.textContent = "";
	if (list.length === 0) {
		rows.appendChild(messageRow("No live reports."));
		return;
	}
	for (const report of list) {
		rows.appendChild(reportRow(report));
	}
}

// showReportsError replaces the report rows with one error row, so a
// failed fetch never leaves the table blank.
function showReportsError(err) {
	const rows = document.getElementById("report-rows");
	rows.textContent = "";
	rows.appendChild(messageRow("error: " + err.message));
}

function loadReports() {
	const rows = document.getElementById("report-rows");
	if (rows === null) return;
	rows.textContent = "";
	rows.appendChild(messageRow("Loading..."));
	fetchJSON("GET", "/api/reports")
		.then((doc) => {
			fetchedReports = doc.reports || [];
			applyReportFilter();
		})
		.catch(showReportsError);
}

// Per-row delete: its own cell, and the button stops click propagation
// so clicking it never toggles the row's expansion. Delete is two-step
// (click, then confirm): the first click arms the confirmation and only
// the confirm control issues the DELETE - against the report's identity,
// rule and target - after which the list refetches so the deleted Report
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
	if (invalid) {
		renderReports(fetchedReports);
		return;
	}
	// Clearing the box restores the full list.
	if (match === null) {
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
		rows.appendChild(messageRow("No reports match the filter."));
		return;
	}
	renderReports(narrowed);
}

// applyStatsFilter narrows the visible key/value rows to the search
// box's matches - over key and value, unanchored and
// case-insensitive - so the search box behaves the same whichever
// view is on screen. An invalid pattern leaves the full rows visible.
function applyStatsFilter() {
	if (document.getElementById("group-identity") === null) return;
	const { match, invalid } = searchMatcher();
	renderStatsGroups(fetchedStatsGroups, invalid ? null : match);
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
