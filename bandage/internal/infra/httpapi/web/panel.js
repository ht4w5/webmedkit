"use strict";

// Web Panel shell: tab switching plus the Statistic tab's manual reload.
// Data comes from the machine API only; nothing here polls - the Refresh
// button is the only reload trigger.

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

document.getElementById("refresh").addEventListener("click", loadStatistic);

for (const tab of document.querySelectorAll(".tab")) {
	tab.addEventListener("click", () => showTab(tab.getAttribute("data-tab")));
}

loadStatistic();
