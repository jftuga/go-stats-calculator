"use strict";

// theme.js runs in the report's <head>, before the body is parsed, and puts
// the remembered theme and width on the root element so the first paint
// already uses them: a dark choice never flashes light, and charts are first
// drawn at the chosen width. report.js wires the buttons that change them.
// Storage can be missing on file:// pages, so every access is inside try.
(() => {
  try {
    const root = document.documentElement;
    const theme = localStorage.getItem("stats-report-theme");
    if (theme === "light" || theme === "dark") {
      root.dataset.theme = theme;
    }
    const width = localStorage.getItem("stats-report-width");
    if (width === "wide" || width === "full") {
      root.dataset.width = width;
    }
  } catch {
    // No storage: the page follows the OS theme at Normal width.
  }
})();
