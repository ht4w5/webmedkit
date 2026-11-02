"use strict";

// Web Panel: tab switching plus the two tabs' manual reloads - the
// Statistic snapshot and the report review table. Data comes from the
// machine API only; nothing here polls - the Refresh button is the only
// reload trigger.

function showTab(name) {
	for (const tab of document.querySelectorAll(".tab")) {
		const active = tab.getAttribute("data-tab") === name;
		tab.classList.toggle("active", active);
		tab.setAttribute("aria-selected", active ? "true" : "false");
	}
	document.getElementById("panel-statistic").hidden = name !== "statistic";
	document.getElementById("panel-reports").hidden = name !== "reports";
}

function loadStatistic() {
	const out = document.getElementById("statistic");
	out.textContent = "Loading...";
	fetch("/api/stats")
		.then((res) => {
			if (!res.ok) {
				throw new Error("GET /api/stats returned HTTP " + res.status);
			}
			return res.json();
		})
		.then((snapshot) => {
			out.textContent = JSON.stringify(snapshot, null, 2);
		})
		.catch((err) => {
			out.textContent = "error: " + err.message;
		});
}

// Report review table: one row per live Report fetched from
// /api/reports. The fetched list is rendered by renderReports(list), so
// a filter can re-render a narrowed list later, and reportRow(report)
// builds one row's markup, so a per-row delete button can join it later.

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
	const detail = document.createElement("tr");
	detail.className = "report-detail";
	detail.hidden = true;
	const td = document.createElement("td");
	td.colSpan = 4;
	const pre = document.createElement("pre");
	pre.className = "json";
	// The full report body pretty-printed: reason, penalty, sample,
	// expiry and the rest of the entry.
	pre.textContent = JSON.stringify(report, null, 2);
	td.appendChild(pre);
	detail.appendChild(td);
	// The toggle reaches only this row's own detail, so expanding one
	// row never disturbs the others. (A per-row delete button must stop
	// click propagation so it does not toggle the row.)
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
	td.colSpan = 4;
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

function loadReports() {
	const rows = document.getElementById("report-rows");
	rows.textContent = "";
	rows.appendChild(messageRow("Loading..."));
	fetch("/api/reports")
		.then((res) => {
			if (!res.ok) {
				throw new Error("GET /api/reports returned HTTP " + res.status);
			}
			return res.json();
		})
		.then((doc) => {
			fetchedReports = doc.reports || [];
			applyReportFilter();
		})
		.catch((err) => {
			rows.textContent = "";
			rows.appendChild(messageRow("error: " + err.message));
		});
}

// Report filter: one regex box narrowing the fetched Reports
// client-side - an unanchored, case-insensitive search across rule,
// target, and the stringified body. This is a panel convenience over
// stored Reports, deliberately independent of the Filter DSL's regex
// matchers, which evaluate Events during rule evaluation.

// fetchedReports holds the full list from the last fetch; the filter
// only ever re-renders a narrowed view of it.
let fetchedReports = [];

function applyReportFilter() {
	const pattern = document.getElementById("report-filter").value;
	const error = document.getElementById("report-filter-error");
	let match = null;
	if (pattern !== "") {
		try {
			match = new RegExp(pattern, "i");
		} catch (err) {
			// A typo never blanks the view: report the bad pattern
			// inline and leave the full list visible until it is fixed.
			error.textContent = "invalid pattern: " + err.message;
			error.hidden = false;
			renderReports(fetchedReports);
			return;
		}
	}
	// Clearing the box restores the full list.
	error.hidden = true;
	error.textContent = "";
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
		const rows = document.getElementById("report-rows");
		rows.textContent = "";
		rows.appendChild(messageRow("No reports match the filter."));
		return;
	}
	renderReports(narrowed);
}

// refreshAll is the one manual reload trigger: both tabs refetch on
// Refresh; nothing here polls.
function refreshAll() {
	loadStatistic();
	loadReports();
}

document.getElementById("refresh").addEventListener("click", refreshAll);
document.getElementById("report-filter").addEventListener("input", applyReportFilter);

for (const tab of document.querySelectorAll(".tab")) {
	tab.addEventListener("click", () => showTab(tab.getAttribute("data-tab")));
}

refreshAll();
