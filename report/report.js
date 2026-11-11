"use strict";

// Reads the statistics payload Go embedded in the page. Later sections build
// their charts from this object with createElementNS and textContent only.
function loadReportData() {
  const block = document.getElementById("report-data");
  return JSON.parse(block.textContent);
}

const reportData = loadReportData();
