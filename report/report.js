"use strict";

// Space between a label and its popover, and the minimum margin kept between
// a popover and the viewport edge, in CSS pixels.
const POPOVER_GAP = 8;
const VIEWPORT_MARGIN = 8;

// Reads the statistics payload Go embedded in the page. Later sections build
// their charts from this object with createElementNS and textContent only.
function loadReportData() {
  const block = document.getElementById("report-data");
  return JSON.parse(block.textContent);
}

// Limits value to the range [low, high], preferring low when the range is
// empty (a popover larger than the viewport is pinned to the top left).
function clamp(value, low, high) {
  return Math.max(low, Math.min(value, high));
}

// Places a definition popover beside its label button: to the right when it
// fits, otherwise below (or above when there is no room below), and always
// inside the viewport. The popover is position: fixed in the top layer.
function positionPopover(button, popover) {
  const b = button.getBoundingClientRect();
  const p = popover.getBoundingClientRect();
  const viewWidth = document.documentElement.clientWidth;
  const viewHeight = document.documentElement.clientHeight;

  let left = b.right + POPOVER_GAP;
  let top = b.top;
  if (left + p.width > viewWidth - VIEWPORT_MARGIN) {
    left = b.left;
    top = b.bottom + POPOVER_GAP;
    if (top + p.height > viewHeight - VIEWPORT_MARGIN) {
      top = b.top - POPOVER_GAP - p.height;
    }
  }
  left = clamp(left, VIEWPORT_MARGIN, viewWidth - VIEWPORT_MARGIN - p.width);
  top = clamp(top, VIEWPORT_MARGIN, viewHeight - VIEWPORT_MARGIN - p.height);
  popover.style.inset = "auto";
  popover.style.left = left + "px";
  popover.style.top = top + "px";
}

// Connects every label button to its popover. Opening, closing, light
// dismiss and Esc come from the popover attribute; this only positions the
// open popover and keeps it beside its label on scroll and resize.
function initDefinitions() {
  let open = null;
  for (const button of document.querySelectorAll("button[popovertarget]")) {
    const popover = document.getElementById(button.getAttribute("popovertarget"));
    if (!popover) {
      continue;
    }
    // The popover has no size until it is shown, which happens right after
    // this event; an animation frame measures it before it is first painted.
    popover.addEventListener("beforetoggle", (event) => {
      if (event.newState === "open") {
        open = { button, popover };
        requestAnimationFrame(() => positionPopover(button, popover));
      } else if (open && open.popover === popover) {
        open = null;
      }
    });
    // The toggle event places it again where animation frames are throttled.
    popover.addEventListener("toggle", (event) => {
      if (event.newState === "open") {
        positionPopover(button, popover);
      }
    });
  }
  const reposition = () => {
    if (open) {
      positionPopover(open.button, open.popover);
    }
  };
  window.addEventListener("resize", reposition);
  window.addEventListener("scroll", reposition, { passive: true });
}

const reportData = loadReportData();
initDefinitions();
