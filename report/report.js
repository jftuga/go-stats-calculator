"use strict";

// report.js draws the stats report's interactive parts from the JSON data
// block Go embeds in the page: definition popovers, and charts built as SVG
// with createElementNS and textContent only (no markup is ever parsed from a
// string), so no value or file name can be interpreted as markup. Sections, in order: shared helpers,
// definitions, the chart kit (SVG, scales, axes, labels, tooltip, animation,
// mounting, x-y plots), the charts of report sections 1 to 4, and startup.

// ===== Shared helpers =====================================================

// Space between a label and its popover, and the minimum margin kept between
// a popover or tooltip and the viewport edge, in CSS pixels.
const POPOVER_GAP = 8;
const VIEWPORT_MARGIN = 8;

// Reads the statistics payload Go embedded in the page.
function loadReportData() {
  const block = document.getElementById("report-data");
  return JSON.parse(block.textContent);
}

// Limits value to the range [low, high], preferring low when the range is
// empty (a popover larger than the viewport is pinned to the top left).
function clamp(value, low, high) {
  return Math.max(low, Math.min(value, high));
}

// ===== Definitions ========================================================

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

// ===== Chart kit: elements ================================================

const SVG_NS = "http://www.w3.org/2000/svg";

// Layout constants shared by every chart, in CSS pixels.
const CHART_MARGIN = 10; // horizontal room kept at each side of a plot
const TICK_SPACING = 80; // target distance between axis ticks
const MAX_TICK_CHARS = 9; // longest plain tick label before offsets are used
const LANE_HEIGHT = 15; // vertical step between stacked label lanes
const LABEL_GAP = 8; // minimum horizontal gap between labels in one lane
const CHAR_WIDTH = 6.5; // text width estimate when measuring is impossible

// Creates an SVG element with the given attributes and appends it to parent.
function svgEl(parent, tag, attrs) {
  const el = document.createElementNS(SVG_NS, tag);
  for (const [name, value] of Object.entries(attrs || {})) {
    el.setAttribute(name, String(value));
  }
  parent.appendChild(el);
  return el;
}

// Creates an SVG text element at (x, y) holding text as plain text.
function svgText(parent, x, y, text, attrs) {
  const el = svgEl(parent, "text", Object.assign({ x, y }, attrs));
  el.textContent = text;
  return el;
}

// Creates an HTML element with a class and plain-text content.
function htmlEl(parent, tag, className, text) {
  const el = document.createElement(tag);
  el.className = className;
  if (text !== undefined) {
    el.textContent = text;
  }
  parent.appendChild(el);
  return el;
}

// Returns the rendered width of an SVG text element, estimated from its
// length when it cannot be measured (a hidden container).
function textWidth(el) {
  const width = el.getComputedTextLength();
  return width > 0 ? width : el.textContent.length * CHAR_WIDTH;
}

// Starts a chart's SVG at the container's width; text inside can be measured
// right away. finishSvg sets the height once the layout is known.
function newSvg(container, width) {
  return svgEl(container, "svg", { width, height: 1, "aria-hidden": "true" });
}

// Gives a chart's SVG its final height and a matching viewBox.
function finishSvg(svg, width, height) {
  svg.setAttribute("height", height);
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
}

// The name each chart goes by in the line that replaces it, keyed by its
// data-chart name.
const CHART_NAMES = {
  distinct: "Duplicate bar",
  range: "Range strip",
  center: "Number line",
  box: "Box plot",
  ladder: "Percentile ladder",
  skewness: "Skewness gauge",
  kurtosis: "Kurtosis gauge",
  cv: "CV gauge",
  histogram: "Histogram",
  ecdf: "ECDF",
  qq: "Q-Q plot",
  trendline: "Trendline",
  lag: "Lag-1 plot",
};

// Shows "<Chart> not shown: <reason>." in place of the chart of container (a
// [data-chart] element) when it has nothing to draw.
function showReason(container, reason) {
  htmlEl(container, "p", "chart-reason", `${CHART_NAMES[container.dataset.chart]} not shown: ${reason}.`);
}

// Adds a line of explanation under a chart; warning lines are emphasized.
function showNote(container, text, warning) {
  htmlEl(container, "p", warning ? "chart-warning" : "chart-note", text);
}

// ===== Chart kit: scales, ticks and number formatting =====================

// Returns a function mapping domain [d0, d1] linearly onto range [r0, r1].
// An empty domain maps everything to the middle of the range.
function linearScale(domain, range) {
  const [d0, d1] = domain;
  const [r0, r1] = range;
  const span = d1 - d0;
  const scale = (v) => (span > 0 ? r0 + ((v - d0) / span) * (r1 - r0) : (r0 + r1) / 2);
  scale.domain = domain;
  scale.range = range;
  return scale;
}

// Widens [lo, hi] by fraction of its span on each side.
function padDomain(lo, hi, fraction) {
  const pad = (hi - lo) * fraction;
  return [lo - pad, hi + pad];
}

// Reports whether lo and hi differ by more than floating-point noise, so a
// span of a few ulps (ten copies of 0.1 have a mean just under 0.1) is not
// drawn as a meaningful range.
function hasSpan(lo, hi) {
  return hi - lo > 1e-12 * Math.max(Math.abs(lo), Math.abs(hi));
}

// Returns about count tick values on [lo, hi] at a step of 1, 2 or 5 times a
// power of ten, and at least minStep (1 keeps counts and positions whole).
// Values are i * step so they do not accumulate rounding error.
function niceTicks(lo, hi, count, minStep) {
  const raw = (hi - lo) / Math.max(1, count);
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const norm = raw / magnitude;
  const step = Math.max((norm < 1.5 ? 1 : norm < 3 ? 2 : norm < 7 ? 5 : 10) * magnitude, minStep || 0);
  const values = [];
  for (let i = Math.ceil(lo / step); i * step <= hi + step * 1e-9; i++) {
    values.push(i * step);
  }
  return { values, step };
}

// Removes trailing zeros after a decimal point, and the point itself.
function trimZeros(text) {
  if (!text.includes(".")) {
    return text;
  }
  // \.?0+$ matches the trailing zeros and, when nothing else follows the
  // point, the point too.
  return text.replace(/\.?0+$/, "");
}

// Uses a true minus sign (U+2212) for a number shown in a chart.
function minusSign(text) {
  return text.replaceAll("-", "−");
}

// Gives every number Go preformats for the charts (each "text" field of the
// summary and distribution payloads, all formatFloat output) a true minus
// sign, like the labels the page formats itself. The table is rendered by the
// template from its own copy, so it keeps the text output's hyphen.
function useMinusSigns(node) {
  for (const [key, value] of Object.entries(node)) {
    if (key === "text" && typeof value === "string") {
      node[key] = minusSign(value);
    } else if (value && typeof value === "object") {
      useMinusSigns(value);
    }
  }
}

// Names a data value on an axis: ln(value) under -l (the flag behind the
// header's -l chip and banner), so a logged value is never read as raw.
function valueName(data) {
  return data.stats.logTransformed ? "ln(value)" : "value";
}

// Labels a data value's text in a tooltip: "ln(value) 2.3026" under -l,
// otherwise word and the text ("value 10"), or the text alone with no word.
function valueText(data, text, word) {
  const name = data.stats.logTransformed ? valueName(data) : word;
  return name ? `${name} ${text}` : text;
}

// Qualifies data values quoted in a chart note: ", as ln(value)" under -l,
// otherwise nothing.
function lnSuffix(data) {
  return data.stats.logTransformed ? `, as ${valueName(data)}` : "";
}

// Unit suffixes for large tick values, largest first.
const TICK_UNITS = [
  [1e12, "T"],
  [1e9, "B"],
  [1e6, "M"],
  [1e3, "k"],
];

// Most decimals a tick or value is written with before switching to
// exponent notation (toFixed rejects more than 100; values near 1e-170 have
// spread).
const MAX_TICK_DECIMALS = 12;
const MAX_VALUE_DECIMALS = 100;

// Formats v in exponent notation with digits after the point, trailing zeros
// of the mantissa trimmed: 1.5e-170.
function exponentText(v, digits) {
  const [mantissa, exponent] = v.toExponential(digits).split("e");
  return `${trimZeros(mantissa)}e${exponent}`.replaceAll("-", "−");
}

// Formats an axis tick with just enough decimals for its step. Large values
// with a coarse step use a k, M, B or T suffix when that needs at most two
// decimals; otherwise every digit is kept, and drawAxis switches to offsets
// when the result is too long. A step too fine for plain decimals gives
// exponent notation with enough digits to tell neighboring ticks apart.
function formatTick(v, step, maxAbs) {
  if (v === 0) {
    return "0";
  }
  if (-Math.log10(step) > MAX_TICK_DECIMALS) {
    return exponentText(v, clamp(Math.ceil(Math.log10(Math.abs(v) / step)), 0, 20));
  }
  if (step >= 1e3 && maxAbs >= 1e5) {
    const [unit, suffix] = TICK_UNITS.find(([u]) => maxAbs >= u);
    const decimals = Math.max(0, Math.ceil(-Math.log10(step / unit)));
    if (decimals <= 2) {
      return minusSign(trimZeros((v / unit).toFixed(decimals)) + suffix);
    }
  }
  return minusSign(v.toFixed(Math.max(0, Math.ceil(-Math.log10(step)))));
}

// Returns the tick labels for values at step. When the plain labels are too
// long to sit side by side because the values share three or more leading
// digits (epoch timestamps, 1e15 plus a small spread), the labels become
// offsets from the first tick, returned as base. Long labels on small values
// stay plain, since an offset would be just as long.
function tickLabels(values, step, maxAbs) {
  const plain = values.map((v) => formatTick(v, step, maxAbs));
  const base = values[0];
  const span = values[values.length - 1] - base;
  if (values.length < 2 || span >= 1e-3 * maxAbs || Math.max(...plain.map((t) => t.length)) <= MAX_TICK_CHARS) {
    return { texts: plain, base: null };
  }
  return { texts: values.map((v) => "+" + formatTick(v - base, step, span)), base };
}

// Formats a value the way Go's formatFloat does: 4 decimals from magnitude 1
// up, one more per leading zero below that, trailing zeros trimmed, with a
// true minus sign. Used only for values Go does not send as text (fences,
// whiskers, bin centers).
function formatValue(v) {
  if (Number.isInteger(v)) {
    return minusSign(v.toFixed(0));
  }
  const a = Math.abs(v);
  const decimals = a >= 1 ? 4 : 4 - Math.floor(Math.log10(a)) - 1;
  if (decimals > MAX_VALUE_DECIMALS) {
    return exponentText(v, 3);
  }
  return minusSign(trimZeros(v.toFixed(decimals)));
}

// Draws a horizontal axis at y for scale: a baseline, ticks and labels kept
// inside the SVG, with a note under the labels when they are offsets. Ticks
// are thinned to every other one until no two labels overlap, and are at
// least minStep apart. Returns the y just below the labels or the note.
function drawAxis(svg, scale, y, width, minStep) {
  const [r0, r1] = scale.range;
  const [d0, d1] = scale.domain;
  const g = svgEl(svg, "g", { class: "axis" });
  svgEl(g, "line", { x1: r0, x2: r1, y1: y, y2: y });
  let { values, step } = niceTicks(d0, d1, Math.max(2, Math.floor((r1 - r0) / TICK_SPACING)), minStep);
  const maxAbs = Math.max(Math.abs(d0), Math.abs(d1));
  for (;;) {
    const { texts, base } = tickLabels(values, step, maxAbs);
    const ticks = svgEl(g, "g");
    let prevRight = -Infinity;
    let overlap = false;
    values.forEach((v, i) => {
      const x = scale(v);
      svgEl(ticks, "line", { x1: x, x2: x, y1: y, y2: y + 4 });
      const label = svgText(ticks, x, y + 15, texts[i], { "text-anchor": "middle" });
      const w = textWidth(label);
      const left = clamp(x - w / 2, 0, Math.max(0, width - w));
      label.setAttribute("x", left + w / 2);
      overlap ||= left < prevRight + LABEL_GAP;
      prevRight = left + w;
    });
    if (overlap && values.length > 2) {
      ticks.remove();
      values = values.filter((_, i) => i % 2 === 0);
      step *= 2;
      continue;
    }
    if (base === null) {
      return y + 20;
    }
    svgText(g, r0, y + 29, `ticks are offsets from ${formatValue(base)}`, { class: "muted" });
    return y + 34;
  }
}

// Draws a horizontal axis's title right-aligned at x under its labels, which
// end at bottom. Returns the y below the title.
function axisTitle(svg, x, bottom, text) {
  svgText(svg, x, bottom + 10, text, { class: "muted", "text-anchor": "end" });
  return bottom + 14;
}

// Draws the value axis of a chart that has no axis title (number line, box
// plot, ladder). Under -l it gains an ln(value) title, so its ticks are not
// read as raw values. Returns the y below everything drawn.
function drawValueAxis(svg, data, scale, y, width) {
  const bottom = drawAxis(svg, scale, y, width);
  return data.stats.logTransformed ? axisTitle(svg, scale.range[1], bottom, valueName(data)) : bottom;
}

// ===== Chart kit: label lanes =============================================

// Assigns each item ({x, w}) a left edge centered on x but kept inside
// [0, width], and the lowest lane where it does not overlap the previous
// label in that lane. Returns the number of lanes used.
function layoutLanes(items, width) {
  const laneEnds = [];
  for (const item of [...items].sort((a, b) => a.x - b.x)) {
    item.left = clamp(item.x - item.w / 2, 0, Math.max(0, width - item.w));
    let lane = laneEnds.findIndex((end) => end + LABEL_GAP <= item.left);
    if (lane < 0) {
      lane = laneEnds.length;
      laneEnds.push(0);
    }
    laneEnds[lane] = item.left + item.w;
    item.lane = lane;
  }
  return laneEnds.length;
}

// Creates a haloed label for each spec ({x, text}) in group, measures it and
// assigns lanes. Returns the items and the lane count; positionLabels places
// them once the caller knows where the lanes go.
function measureLabels(group, specs, width) {
  const items = specs.map((spec) => {
    const el = svgText(group, 0, 0, spec.text, { class: "halo" });
    return { x: spec.x, w: textWidth(el), el };
  });
  return { items, lanes: layoutLanes(items, width) };
}

// Moves measured labels to their lanes, baselineOf(lane) giving each lane's
// text baseline. A dashed leader joins a label to its point at anchorY when
// the label is not directly beside it.
function positionLabels(items, leaders, anchorY, baselineOf) {
  for (const item of items) {
    const y = baselineOf(item.lane);
    item.el.setAttribute("x", item.left);
    item.el.setAttribute("y", y);
    const covers = item.x >= item.left && item.x <= item.left + item.w;
    if (item.lane > 0 || !covers) {
      const end = y < anchorY ? y + 3 : y - 10;
      const x2 = clamp(item.x, item.left, item.left + item.w);
      svgEl(leaders, "line", { class: "leader", x1: item.x, x2, y1: anchorY, y2: end });
    }
  }
}

// ===== Chart kit: tooltip =================================================

// The single tooltip element shared by every chart, created on first use.
let tooltipEl = null;

// Shows the tooltip with one line per non-empty string, near the pointer.
function showTooltip(lines, clientX, clientY) {
  if (!tooltipEl) {
    tooltipEl = htmlEl(document.body, "div", "chart-tip");
    tooltipEl.setAttribute("role", "tooltip");
  }
  tooltipEl.replaceChildren();
  for (const line of lines) {
    if (line) {
      htmlEl(tooltipEl, "div", "", line);
    }
  }
  tooltipEl.hidden = false;
  moveTooltip(clientX, clientY);
}

// Places the visible tooltip beside the pointer, flipping to the other side
// of it near an edge and always staying inside the viewport.
function moveTooltip(clientX, clientY) {
  if (!tooltipEl || tooltipEl.hidden) {
    return;
  }
  const r = tooltipEl.getBoundingClientRect();
  const viewWidth = document.documentElement.clientWidth;
  const viewHeight = document.documentElement.clientHeight;
  let left = clientX + 12;
  let top = clientY + 14;
  if (left + r.width > viewWidth - VIEWPORT_MARGIN) {
    left = clientX - 12 - r.width;
  }
  if (top + r.height > viewHeight - VIEWPORT_MARGIN) {
    top = clientY - 14 - r.height;
  }
  tooltipEl.style.left = clamp(left, VIEWPORT_MARGIN, viewWidth - VIEWPORT_MARGIN - r.width) + "px";
  tooltipEl.style.top = clamp(top, VIEWPORT_MARGIN, viewHeight - VIEWPORT_MARGIN - r.height) + "px";
}

// Hides the tooltip.
function hideTooltip() {
  if (tooltipEl) {
    tooltipEl.hidden = true;
  }
}

// Shows lines in the tooltip while the pointer is over target.
function addTooltip(target, lines) {
  target.addEventListener("pointerenter", (e) => showTooltip(lines, e.clientX, e.clientY));
  target.addEventListener("pointermove", (e) => moveTooltip(e.clientX, e.clientY));
  target.addEventListener("pointerleave", hideTooltip);
}

// ===== Chart kit: scroll-in animation =====================================

const ANIMATION_MS = 700;
const EASE_OUT = "cubic-bezier(0.2, 0.7, 0.2, 1)";
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

// Returns a collector for one chart's animations. Charts are always built in
// their final state; when enabled, add() starts an animation paused at its
// first frame, and playOnFirstView runs them all once. When disabled
// (reduced motion, or a redraw after resizing) add() does nothing, so the
// final state shows immediately.
function createMotion(enabled) {
  const animations = [];
  return {
    animations,
    add(el, keyframes, options) {
      if (!enabled || typeof el.animate !== "function") {
        return;
      }
      const timing = Object.assign({ duration: ANIMATION_MS, easing: EASE_OUT, fill: "backwards" }, options);
      const animation = el.animate(keyframes, timing);
      animation.pause();
      animations.push(animation);
    },
  };
}

// Plays animations once, when element first scrolls into view. Returns the
// observer so a redraw can cancel it, or null.
function playOnFirstView(element, animations) {
  if (animations.length === 0) {
    return null;
  }
  const play = () => animations.forEach((a) => a.play());
  if (typeof IntersectionObserver !== "function") {
    play();
    return null;
  }
  const observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) {
        observer.disconnect();
        play();
      }
    },
    { threshold: 0.2 },
  );
  observer.observe(element);
  return observer;
}

// Slides el horizontally from dx pixels away into its place.
function slideIn(motion, el, dx, delay) {
  motion.add(el, [{ transform: `translate(${dx}px, 0px)` }, { transform: "translate(0px, 0px)" }], { delay: delay || 0 });
}

// Grows el horizontally out of the point (originX, originY).
function growIn(motion, el, originX, originY, delay) {
  el.style.transformOrigin = `${originX}px ${originY}px`;
  motion.add(el, [{ transform: "scaleX(0)" }, { transform: "scaleX(1)" }], { delay: delay || 0 });
}

// Fades el in after delay milliseconds.
function fadeIn(motion, el, delay) {
  motion.add(el, [{ opacity: 0 }, { opacity: 1 }], { duration: 350, delay: delay || 0 });
}

// Grows el vertically out of the point (originX, originY), for bars rising
// from a baseline.
function growUp(motion, el, originX, originY, delay) {
  el.style.transformOrigin = `${originX}px ${originY}px`;
  motion.add(el, [{ transform: "scaleY(0)" }, { transform: "scaleY(1)" }], { delay: delay || 0 });
}

// Uncovers el from left to right, for lines drawn in input order.
function revealLeftToRight(motion, el, delay) {
  motion.add(el, [{ clipPath: "inset(0 100% 0 0)" }, { clipPath: "inset(0 0% 0 0)" }], { duration: 1400, easing: "ease-in-out", delay: delay || 0 });
}

// ===== Chart kit: mounting ================================================

// Draws the chart of one container from scratch at its current width. A
// drawing error replaces only that chart with a reason line.
function renderMount(mount, data, animate) {
  const width = Math.floor(mount.el.clientWidth);
  if (width <= 0 || width === mount.width) {
    return;
  }
  mount.width = width;
  if (mount.observer) {
    mount.observer.disconnect();
    mount.observer = null;
  }
  mount.el.replaceChildren();
  const motion = createMotion(animate);
  try {
    mount.draw({ el: mount.el, data, width, motion, name: mount.el.dataset.chart });
  } catch (err) {
    mount.el.replaceChildren();
    showReason(mount.el, "drawing it failed");
    console.error(err);
    return;
  }
  mount.observer = playOnFirstView(mount.el, motion.animations);
}

// Draws every [data-chart] container with its drawing function, and redraws
// them without animation when the window is resized.
function mountCharts(data, drawers) {
  const mounts = [];
  for (const el of document.querySelectorAll("[data-chart]")) {
    const draw = drawers[el.dataset.chart];
    if (draw) {
      mounts.push({ el, draw, width: 0, observer: null });
    }
  }
  for (const mount of mounts) {
    renderMount(mount, data, !reducedMotion.matches);
  }
  let timer = 0;
  window.addEventListener("resize", () => {
    hideTooltip();
    clearTimeout(timer);
    timer = setTimeout(() => mounts.forEach((m) => renderMount(m, data, false)), 150);
  });
  window.addEventListener("scroll", hideTooltip, { passive: true });
}

// ===== Section 1: Descriptive Statistics ==================================

// Draws the duplicate bar: the share of distinct values against the share
// that repeat an earlier value.
function drawDistinctBar({ el, data, width, motion }) {
  const d = data.summary.distinct;
  const duplicated = d.count - d.distinct;
  const svg = newSvg(el, width);
  const x = linearScale([0, d.count], [0, width]);
  const split = duplicated > 0 ? clamp(x(d.distinct), 1, width - 1) : width;

  const bar = svgEl(svg, "g");
  const distinct = svgEl(bar, "rect", { class: "seg-distinct", x: 0, y: 2, width: split, height: 10 });
  addTooltip(distinct, ["Distinct values", `${d.distinct} of ${d.count}`]);
  if (duplicated > 0) {
    const dup = svgEl(bar, "rect", { class: "seg-dup", x: split, y: 2, width: width - split, height: 10 });
    addTooltip(dup, ["Duplicated values", `${duplicated} of ${d.count} (${d.duplicatedText}%) repeat an earlier value`]);
  }
  growIn(motion, bar, 0, 7);

  let height = 32;
  if (duplicated === 0) {
    svgText(svg, 0, 26, d.count === 1 ? "1 value" : `all ${d.count} distinct`);
  } else {
    const left = svgText(svg, 0, 26, `${d.distinct} distinct`);
    const right = svgText(svg, width, 26, `${duplicated} duplicated (${d.duplicatedText}%)`, { "text-anchor": "end" });
    if (textWidth(left) + textWidth(right) + LABEL_GAP > width) {
      right.setAttribute("y", 41);
      height = 47;
    }
  }
  finishSvg(svg, width, height);
}

// Draws the min-to-max range strip with the Q1 to Q3 box and ticks at Q1,
// the median and Q3, labeled below in lanes.
function drawRangeStrip({ el, data, width, motion }) {
  const marks = data.summary.range;
  if (!marks) {
    showReason(el, "every value is the same");
    return;
  }
  const m = Object.fromEntries(marks.map((mark) => [mark.key, mark]));
  const svg = newSvg(el, width);
  const x = linearScale([m.min.value, m.max.value], [CHART_MARGIN, width - CHART_MARGIN]);
  const y = 16;

  const strip = svgEl(svg, "g");
  svgEl(strip, "line", { class: "strip-line", x1: x(m.min.value), x2: x(m.max.value), y1: y, y2: y });
  const box = svgEl(strip, "rect", {
    class: "strip-box",
    x: x(m.q1.value),
    y: y - 7,
    width: Math.max(1, x(m.q3.value) - x(m.q1.value)),
    height: 14,
  });
  addTooltip(box, ["Q1 to Q3", valueText(data, `[${m.q1.text}, ${m.q3.text}]`), "The middle half of the values"]);
  for (const mark of marks) {
    const mx = x(mark.value);
    const reach = mark.key === "median" ? 11 : 7;
    svgEl(strip, "line", { class: mark.key === "median" ? "median-line" : "strip-tick", x1: mx, x2: mx, y1: y - reach, y2: y + reach });
    addTooltip(svgEl(strip, "rect", { class: "hit", x: mx - 5, y: y - 12, width: 10, height: 24 }), [mark.label, valueText(data, mark.text)]);
  }
  growIn(motion, strip, x(m.min.value), y);

  const leaders = svgEl(svg, "g");
  const labels = svgEl(svg, "g");
  const specs = marks.map((mark) => ({ x: x(mark.value), text: `${mark.label} ${mark.text}` }));
  const { items, lanes } = measureLabels(labels, specs, width);
  const top = y + 26;
  positionLabels(items, leaders, y + 12, (lane) => top + lane * LANE_HEIGHT);
  fadeIn(motion, leaders, 450);
  fadeIn(motion, labels, 450);
  const bottom = top + (lanes - 1) * LANE_HEIGHT + 6;
  // The strip has no axis, so under -l its labels need their own title.
  finishSvg(svg, width, data.stats.logTransformed ? axisTitle(svg, width, bottom, valueName(data)) : bottom);
}

// ===== Section 2: Measures of Central Tendency ============================

// Draws a center's marker centered on (0, 0); the shape tells the centers
// apart without relying on color.
function centerMarker(parent, key) {
  const cls = `marker mk-${key}`;
  switch (key) {
    case "median":
      return svgEl(parent, "path", { class: cls, d: "M0,-6L6,0L0,6L-6,0Z" });
    case "mode":
      return svgEl(parent, "rect", { class: cls, x: -4.5, y: -4.5, width: 9, height: 9 });
    case "geomean":
      return svgEl(parent, "path", { class: cls, d: "M0,-6L6,5L-6,5Z" });
    case "trimmedmean":
      return svgEl(parent, "path", { class: cls, d: "M0,6L6,-5L-6,-5Z" });
    default:
      return svgEl(parent, "circle", { class: cls, r: 5 });
  }
}

// Narrowest the CI band is drawn, in pixels; at large n the true band is far
// narrower than a pixel.
const MIN_CI_WIDTH = 4;

// Draws the number line of the centers (mean, median, single mode or modal
// bin, geometric and trimmed mean when present) with the 95% CI band. The
// domain spans the markers and the band, padded, with Q1 to Q3 shaded behind
// for scale. A multi-mode dataset's modes are not drawn; a note says so.
function drawCenterLine({ el, data, width, motion }) {
  const c = data.summary.center;
  const xs = c.marks.map((mark) => mark.value);
  if (c.ci) {
    xs.push(c.ci.low, c.ci.high);
  }
  if (c.modalBin) {
    xs.push(c.modalBin.low, c.modalBin.high);
  }
  const lo = Math.min(...xs);
  const hi = Math.max(...xs);
  if (!hasSpan(lo, hi)) {
    showReason(el, `every center is ${valueText(data, c.marks[0].text)}`);
    return;
  }

  const svg = newSvg(el, width);
  const x = linearScale(padDomain(lo, hi, 0.08), [CHART_MARGIN, width - CHART_MARGIN]);
  const [r0, r1] = x.range;
  const shadeLayer = svgEl(svg, "g");
  const lineLayer = svgEl(svg, "g");
  const leaders = svgEl(svg, "g");
  const markLayer = svgEl(svg, "g");
  const labels = svgEl(svg, "g");

  const specs = c.marks.map((mark) => ({ x: x(mark.value), text: `${mark.label} ${mark.text}` }));
  if (c.ci) {
    specs.push({ x: x((c.ci.low + c.ci.high) / 2), text: c.ci.label });
  }
  if (c.modalBin) {
    specs.push({ x: x((c.modalBin.low + c.modalBin.high) / 2), text: `Modal bin (${c.modalBin.bins} bins, -b)` });
  }
  const { items, lanes } = measureLabels(labels, specs, width);
  const lineY = lanes * LANE_HEIGHT + 20;
  positionLabels(items, leaders, lineY - 9, (lane) => lineY - 18 - lane * LANE_HEIGHT);
  fadeIn(motion, leaders, 500);
  fadeIn(motion, labels, 500);
  const axisY = lineY + 20;

  // Q1 to Q3 behind everything, clipped to the view. The median is always a
  // marker and lies between the quartiles, so the shading is never empty.
  const q = c.quartiles;
  const s0 = clamp(x(q.low), r0, r1);
  const s1 = clamp(x(q.high), r0, r1);
  const shade = svgEl(shadeLayer, "rect", { class: "shade", x: s0, y: lineY - 14, width: Math.max(2, s1 - s0), height: axisY - lineY + 14 });
  addTooltip(shade, [q.label, valueText(data, q.text)]);
  const clipped = x(q.low) < r0 || x(q.high) > r1;

  svgEl(lineLayer, "line", { class: "guide", x1: r0, x2: r1, y1: lineY, y2: lineY });
  let ciWidened = false;
  if (c.ci) {
    const mid = x((c.ci.low + c.ci.high) / 2);
    const trueWidth = x(c.ci.high) - x(c.ci.low);
    ciWidened = trueWidth < MIN_CI_WIDTH;
    const w = Math.max(trueWidth, MIN_CI_WIDTH);
    const band = svgEl(lineLayer, "rect", { class: "ci-band", x: mid - w / 2, y: lineY - 8, width: w, height: 16 });
    addTooltip(band, ["95% CI (mean)", valueText(data, c.ci.text), ciWidened ? "Drawn at a minimum width; the true band is narrower" : ""]);
    growIn(motion, band, mid, lineY);
  }
  if (c.modalBin) {
    const b = c.modalBin;
    const bx = x(b.low);
    const bin = svgEl(lineLayer, "rect", { class: "modal-bin", x: bx, y: lineY - 6, width: Math.max(2, x(b.high) - bx), height: 12 });
    const valueWord = b.count === 1 ? "value" : "values";
    addTooltip(bin, ["Modal bin", valueText(data, b.text), `${b.count} ${valueWord}, the densest of ${b.bins} equal-width bins (-b)`, "From the -b binning, so it need not match the histogram's tallest bar"]);
    fadeIn(motion, bin, 300);
  }

  // Markers slide out from the median into place.
  const medianX = x(c.marks.find((mark) => mark.key === "median").value);
  for (const mark of c.marks) {
    const mx = x(mark.value);
    const outer = svgEl(markLayer, "g", { transform: `translate(${mx}, ${lineY})` });
    const inner = svgEl(outer, "g");
    centerMarker(inner, mark.key);
    addTooltip(svgEl(inner, "circle", { class: "hit", r: 9 }), [mark.label, valueText(data, mark.text)]);
    slideIn(motion, inner, medianX - mx);
  }

  const bottom = drawValueAxis(svg, data, x, axisY, width);
  finishSvg(svg, width, bottom + 2);

  showNote(el, `Shaded: Q1 to Q3, ${q.text}${lnSuffix(data)}${clipped ? ", wider than this view" : ""}.`);
  if (ciWidened) {
    showNote(el, "The 95% CI is narrower than a pixel at this scale, so its band is drawn at a minimum width.");
  }
  if (c.modeCount > 1) {
    showNote(el, `Mode not shown: the data has ${c.modeCount} modes, listed in the Mode row.`);
  }
}

// ===== Section 3: Measures of Spread & Distribution =======================

// Draws the Tukey box plot: whiskers at the most extreme values inside the
// k*IQR fences, the fences (when inside the data's range), and the IQR
// outliers as dots, sized by count when there are repeats or bins.
function drawBoxPlot({ el, data, width, motion }) {
  const b = data.charts.box;
  const s = data.stats;
  if (!b) {
    // Go also drops the box when -k is so large that a fence overflows to
    // infinity; then every value is inside the fences, not none.
    const k = s.iqrMultiplier;
    const overflow = !Number.isFinite(s.q1 - k * s.iqr) || !Number.isFinite(s.q3 + k * s.iqr);
    showReason(el, s.min === s.max ? "every value is the same" : overflow ? `-k ${formatValue(k)} puts the fences beyond the largest representable number` : "no value lies inside the fences");
    return;
  }
  const k = formatValue(s.iqrMultiplier);
  const svg = newSvg(el, width);
  const x = linearScale(padDomain(s.min, s.max, 0.03), [CHART_MARGIN, width - CHART_MARGIN]);
  const [d0, d1] = x.domain;
  const y = 34;

  const fences = [
    [b.lowerFence, "Lower fence", `Q1 − ${k} × IQR`],
    [b.upperFence, "Upper fence", `Q3 + ${k} × IQR`],
  ];
  let lastLabel = null; // the previous fence label and its right edge
  let lastEnd = -Infinity;
  for (const [value, name, rule] of fences) {
    if (value < d0 || value > d1) {
      continue;
    }
    const fx = x(value);
    svgEl(svg, "line", { class: "fence", x1: fx, x2: fx, y1: y - 20, y2: y + 20 });
    const label = svgText(svg, fx, y - 24, "fence", { class: "muted", "text-anchor": "middle" });
    const half = textWidth(label) / 2;
    const lx = clamp(fx, half, width - half);
    if (lastLabel && lx - half < lastEnd + LABEL_GAP) {
      // Two fences too close to label apart share the first label.
      label.remove();
      lastLabel.textContent = "fences";
    } else {
      label.setAttribute("x", lx);
      lastLabel = label;
      lastEnd = lx + half;
    }
    addTooltip(svgEl(svg, "rect", { class: "hit", x: fx - 4, y: y - 20, width: 8, height: 40 }), [name, valueText(data, formatValue(value)), rule]);
  }

  const box = svgEl(svg, "g");
  const q1x = x(b.q1);
  const q3x = x(b.q3);
  const lwx = x(b.lowerWhisker);
  const uwx = x(b.upperWhisker);
  for (const [from, to, cap, name, value, where] of [
    [lwx, q1x, lwx, "Lower whisker", b.lowerWhisker, "lowest value inside the lower fence"],
    [q3x, uwx, uwx, "Upper whisker", b.upperWhisker, "highest value inside the upper fence"],
  ]) {
    svgEl(box, "line", { class: "whisker", x1: from, x2: to, y1: y, y2: y });
    svgEl(box, "line", { class: "whisker", x1: cap, x2: cap, y1: y - 8, y2: y + 8 });
    addTooltip(svgEl(box, "rect", { class: "hit", x: Math.min(from, to), y: y - 8, width: Math.max(8, Math.abs(to - from)), height: 16 }), [name, valueText(data, formatValue(value)), `The ${where}`]);
  }
  const rect = svgEl(box, "rect", { class: "box", x: q1x, y: y - 14, width: Math.max(2, q3x - q1x), height: 28 });
  addTooltip(rect, ["Box: Q1 to Q3", valueText(data, `[${formatValue(b.q1)}, ${formatValue(b.q3)}]`), `Median ${valueText(data, formatValue(b.median))}`]);
  const mx = x(b.median);
  svgEl(box, "line", { class: "median-line", x1: mx, x2: mx, y1: y - 14, y2: y + 14 });
  growIn(motion, box, mx, y);

  const o = b.outliers;
  const maxCount = o.counts.reduce((a, c) => Math.max(a, c), 1);
  const dots = svgEl(svg, "g");
  o.values.forEach((v, i) => {
    const count = o.counts[i];
    const r = maxCount > 1 ? 2.5 + 3.5 * Math.sqrt(count / maxCount) : 3;
    const dot = svgEl(dots, "circle", { class: "dot", cx: x(v), cy: y, r });
    const at = valueText(data, formatValue(v));
    const lines = o.binned ? [`${plural(count, "outlier")} near ${at}`] : ["Outlier", at, count > 1 ? `${count} values` : ""];
    addTooltip(dot, lines);
  });
  fadeIn(motion, dots, 450);

  const bottom = drawValueAxis(svg, data, x, y + 28, width);
  finishSvg(svg, width, bottom + 2);

  showNote(el, `Whiskers end at the most extreme values inside the fences, ${k} × IQR beyond the quartiles: [${formatValue(b.lowerFence)}, ${formatValue(b.upperFence)}]${lnSuffix(data)}.`);
  const total = o.counts.reduce((a, c) => a + c, 0);
  if (total === 0) {
    showNote(el, "No IQR outliers.");
  } else if (o.binned) {
    showNote(el, `${total} IQR outliers, binned into ${o.values.length} positions; dot size shows how many values each holds.`);
  } else {
    const repeats = maxCount > 1 ? "; larger dots are repeated values" : "";
    showNote(el, `${plural(total, "IQR outlier")} drawn as dots${repeats}.`);
  }
}

// Draws the percentile ladder: one rung per percentile (Q1, Q3, p95, p99 and
// any -p values), ascending like the rows above it, with a dot at its value.
function drawLadder({ el, data, width, motion }) {
  const rungs = data.summary.ladder;
  const values = rungs.map((r) => r.value);
  const lo = Math.min(...values);
  const hi = Math.max(...values);
  if (!hasSpan(lo, hi)) {
    showReason(el, `every percentile is ${valueText(data, rungs[0].text)}`);
    return;
  }
  const svg = newSvg(el, width);
  const grid = svgEl(svg, "g");
  const path = svgEl(svg, "g");
  const dots = svgEl(svg, "g");
  const texts = svgEl(svg, "g");
  const labels = rungs.map((r) => svgText(texts, 0, 0, r.label));
  const r0 = Math.max(...labels.map(textWidth)) + 14;
  const x = linearScale(padDomain(lo, hi, 0.06), [r0, width - CHART_MARGIN]);
  const rowHeight = 22;
  const top = 4;

  const points = [];
  rungs.forEach((rung, i) => {
    const cy = top + rowHeight * i + rowHeight / 2;
    const cx = x(rung.value);
    points.push(`${cx},${cy}`);
    labels[i].setAttribute("x", 0);
    labels[i].setAttribute("y", cy + 4);
    svgEl(grid, "line", { class: "grid", x1: r0, x2: width - CHART_MARGIN, y1: cy, y2: cy });
    const dot = svgEl(dots, "circle", { class: "dot", cx, cy, r: 4.5 });
    addTooltip(dot, [rung.label, valueText(data, rung.text)]);
    slideIn(motion, dot, r0 - cx, i * 60);
    const value = svgText(texts, cx + 8, cy + 4, rung.text, { class: "halo" });
    if (cx + 8 + textWidth(value) > width) {
      value.setAttribute("x", cx - 8);
      value.setAttribute("text-anchor", "end");
    }
    fadeIn(motion, value, 500 + i * 60);
  });
  const line = svgEl(path, "polyline", { class: "guide", points: points.join(" ") });
  fadeIn(motion, line, 600);

  const bottom = drawValueAxis(svg, data, x, top + rowHeight * rungs.length + 4, width);
  finishSvg(svg, width, bottom + 2);
}

// Gauge extents and band names. The band edges are the interpret* thresholds
// Go sends with each gauge ([t0, t1]); skewness mirrors them about zero. Each
// band is [from, to, name, tone, closed], where a from or to of null is
// open-ended and is drawn to the gauge's end, and closed says the open-ended
// band includes its threshold (|skewness| of exactly 1 reads as high).
const GAUGES = {
  skewness: {
    title: "Skewness",
    unit: "",
    extent: [-2, 2],
    bands: ([t0, t1]) => [
      [null, -t1, "high", "high", true],
      [-t1, -t0, "moderate", "mid"],
      [-t0, t0, "symmetrical", "ok"],
      [t0, t1, "moderate", "mid"],
      [t1, null, "high", "high", true],
    ],
  },
  kurtosis: {
    title: "Kurtosis",
    unit: "",
    extent: [-2, 3],
    bands: ([t0, t1]) => [
      [null, t0, "platykurtic", "mid"],
      [t0, t1, "mesokurtic", "ok"],
      [t1, null, "leptokurtic", "mid"],
    ],
  },
  cv: {
    title: "CV",
    unit: "%",
    extent: [0, 60],
    bands: ([t0, t1]) => [
      [null, t0, "low", "ok"],
      [t0, t1, "moderate", "mid"],
      [t1, null, "high", "high", true],
    ],
  },
};

// Draws a linear gauge for skewness, kurtosis or CV: colored, named bands
// with threshold labels and a needle that slides from 0 to the value. A
// value past either end is clipped to an off-scale arrow with its real value
// printed. An undefined statistic shows N/A and its reason instead.
function drawGauge({ el, data, width, motion, name }) {
  const spec = GAUGES[name];
  const g = data.summary[name];
  const svg = newSvg(el, width);
  const edge = 12; // room for the off-scale arrow
  const x = linearScale(spec.extent, [edge, width - edge]);
  const trackY = 18;
  const trackH = 16;
  const na = Boolean(g.na);
  const unitText = (v) => formatValue(v) + spec.unit;

  const bands = spec.bands(g.thresholds);
  for (const [from, to, label, tone, closed] of bands) {
    const bx = x(from ?? spec.extent[0]);
    const bw = x(to ?? spec.extent[1]) - bx;
    const band = svgEl(svg, "rect", { class: na ? "band-na" : `band-${tone}`, x: bx, y: trackY, width: bw, height: trackH });
    const range =
      from === null ? `${closed ? "≤" : "<"} ${unitText(to)}`
      : to === null ? `${closed ? "≥" : ">"} ${unitText(from)}`
      : `${unitText(from)} to ${unitText(to)}`;
    addTooltip(band, [`${spec.title}: ${label}`, range]);
    const text = svgText(svg, bx + bw / 2, trackY + 12, label, { "text-anchor": "middle", class: na ? "muted" : "" });
    if (textWidth(text) > bw - 4) {
      text.remove();
    }
  }
  const bounds = [...new Set([...spec.extent, ...bands.flatMap(([from, to]) => [from, to])].filter((v) => v !== null))];
  for (const v of bounds) {
    const label = svgText(svg, x(v), trackY + trackH + 13, unitText(v), { class: "muted", "text-anchor": "middle" });
    const half = textWidth(label) / 2;
    label.setAttribute("x", clamp(x(v), half, width - half));
  }
  finishSvg(svg, width, trackY + trackH + 18);

  if (na) {
    svgText(svg, width / 2, 11, "N/A", { class: "muted", "text-anchor": "middle" });
    const placeholder = name === "cv" ? "" : "; the value above is reported as 0";
    showNote(el, `Not defined: ${g.na}${placeholder}.`);
    return;
  }

  const [e0, e1] = spec.extent;
  const off = g.value < e0 ? -1 : g.value > e1 ? 1 : 0;
  const nx = x(clamp(g.value, e0, e1));
  const needle = svgEl(svg, "g", { transform: `translate(${nx}, 0)` });
  const inner = svgEl(needle, "g");
  svgEl(inner, "line", { class: "needle", x1: 0, x2: 0, y1: trackY - 4, y2: trackY + trackH + 3 });
  if (off > 0) {
    svgEl(inner, "path", { class: "needle-head", d: `M3,${trackY + 1}L11,${trackY + trackH / 2}L3,${trackY + trackH - 1}Z` });
  } else if (off < 0) {
    svgEl(inner, "path", { class: "needle-head", d: `M-3,${trackY + 1}L-11,${trackY + trackH / 2}L-3,${trackY + trackH - 1}Z` });
  } else {
    svgEl(inner, "path", { class: "needle-head", d: `M-4,${trackY - 6}L4,${trackY - 6}L0,${trackY - 1}Z` });
  }
  const gaugeText = off ? `${g.text} (off scale)` : g.text;
  addTooltip(svgEl(inner, "rect", { class: "hit", x: -6, y: trackY - 8, width: 12, height: trackH + 12 }), [spec.title, gaugeText, g.reading]);
  slideIn(motion, inner, x(clamp(0, e0, e1)) - nx);

  const label = svgText(svg, nx, trackY - 8, gaugeText, { class: "halo", "text-anchor": "middle" });
  const half = textWidth(label) / 2;
  label.setAttribute("x", clamp(nx, half, width - half));
  fadeIn(motion, label, 500);

  if (g.warning) {
    showNote(el, `Warning: ${g.warning}, so CV may not be meaningful.`, true);
  }
}

// ===== Chart kit: x-y plots ===============================================

const PLOT_HEIGHT = 200; // height of an x-y chart's plotting area
const PLOT_TOP = 22; // room above the plot for the vertical axis title
const Y_TICK_SPACING = 40; // target distance between vertical axis ticks
const POINT_PICK_RADIUS = 12; // farthest a hovered point may be from the pointer

// Creates the vertical axis of an x-y chart for domain [lo, hi] at about one
// tick per Y_TICK_SPACING of height, and measures its labels so the plot can
// start right of them. options.format replaces the tick text (base is then
// null), options.minStep keeps ticks whole, options.title names the axis.
// The grid group is created first so grid lines sit behind the data.
function measureYAxis(svg, [lo, hi], height, options) {
  const grid = svgEl(svg, "g");
  const g = svgEl(svg, "g", { class: "axis" });
  const { values, step } = niceTicks(lo, hi, Math.max(2, Math.floor(height / Y_TICK_SPACING)), options.minStep);
  const maxAbs = Math.max(Math.abs(lo), Math.abs(hi));
  const { texts, base } = options.format ? { texts: values.map(options.format), base: null } : tickLabels(values, step, maxAbs);
  const labels = texts.map((text) => svgText(g, 0, 0, text, { "text-anchor": "end" }));
  if (options.title) {
    svgText(g, 0, PLOT_TOP - 8, options.title);
  }
  return { grid, g, values, labels, base, width: Math.max(0, ...labels.map(textWidth)) };
}

// Places a measured vertical axis at x = left for scale y, with a grid line
// across the plot at each tick.
function placeYAxis(axis, y, left, right) {
  const [r0, r1] = y.range;
  svgEl(axis.g, "line", { x1: left, x2: left, y1: r0, y2: r1 });
  axis.values.forEach((v, i) => {
    const py = y(v);
    svgEl(axis.g, "line", { x1: left - 4, x2: left, y1: py, y2: py });
    svgEl(axis.grid, "line", { class: "grid", x1: left, x2: right, y1: py, y2: py });
    axis.labels[i].setAttribute("x", left - 7);
    axis.labels[i].setAttribute("y", py + 4);
  });
}

// Lays out an x-y chart in svg: the vertical axis for yDomain, then the
// scales, with the plot right of the axis labels. options are those of
// measureYAxis plus height (of the plot) and square (the plot is as wide as
// it is high, never wider than the space available). Returns the scales and
// the plot's edges; yBase is the offset base of the vertical ticks or null.
function plotFrame(svg, width, xDomain, yDomain, options) {
  const opts = options || {};
  const height = opts.height || PLOT_HEIGHT;
  const axis = measureYAxis(svg, yDomain, height, opts);
  const left = Math.ceil(axis.width) + 10;
  let right = width - CHART_MARGIN;
  let plotHeight = height;
  if (opts.square) {
    plotHeight = Math.max(40, Math.min(height, right - left));
    right = left + plotHeight;
  }
  const top = PLOT_TOP;
  const bottom = top + plotHeight;
  const x = linearScale(xDomain, [left, right]);
  const y = linearScale(yDomain, [bottom, top]);
  placeYAxis(axis, y, left, right);
  return { x, y, left, right, top, bottom, yBase: axis.base };
}

// Draws the horizontal axis under a plot with an optional title, sets the
// SVG's height, and notes when the vertical ticks are offsets. Returns the
// y below everything drawn.
function finishPlot(el, svg, frame, width, xTitle, minStep) {
  let bottom = drawAxis(svg, frame.x, frame.bottom, width, minStep);
  if (xTitle) {
    bottom = axisTitle(svg, frame.right, bottom, xTitle);
  }
  if (frame.yBase !== null) {
    showNote(el, `Vertical ticks are offsets from ${formatValue(frame.yBase)}.`);
  }
  return bottom;
}

// Counter for unique clip path ids; charts are redrawn on resize.
let clipCount = 0;

// Returns a clip-path reference that confines an element to the plot area.
function clipToPlot(svg, frame) {
  const id = `plot-clip-${++clipCount}`;
  const clip = svgEl(svgEl(svg, "defs"), "clipPath", { id });
  svgEl(clip, "rect", { x: frame.left, y: frame.top, width: frame.right - frame.left, height: frame.bottom - frame.top });
  return `url(#${id})`;
}

// Covers the plot with one transparent target that shows the tooltip for
// whatever is under the pointer: pick(px, py) receives SVG coordinates and
// returns tooltip lines, or null to hide the tooltip. One target replaces
// thousands of per-point listeners. Call it after drawing, so it is on top.
function addPlotTooltip(svg, frame, pick) {
  const hit = svgEl(svg, "rect", { class: "hit", x: frame.left, y: frame.top, width: frame.right - frame.left, height: frame.bottom - frame.top });
  const update = (e) => {
    const box = svg.getBoundingClientRect();
    const k = box.width > 0 ? Number(svg.getAttribute("width")) / box.width : 1;
    const lines = pick((e.clientX - box.left) * k, (e.clientY - box.top) * k);
    if (lines) {
      showTooltip(lines, e.clientX, e.clientY);
    } else {
      hideTooltip();
    }
  };
  hit.addEventListener("pointerenter", update);
  hit.addEventListener("pointermove", update);
  hit.addEventListener("pointerleave", hideTooltip);
}

// Returns the index of the point (xs[i], ys[i]) nearest to (px, py) within
// POINT_PICK_RADIUS, or -1.
function nearestPoint(xs, ys, px, py) {
  let best = -1;
  let bestDist = POINT_PICK_RADIUS * POINT_PICK_RADIUS;
  for (let i = 0; i < xs.length; i++) {
    const d = (xs[i] - px) ** 2 + (ys[i] - py) ** 2;
    if (d <= bestDist) {
      best = i;
      bestDist = d;
    }
  }
  return best;
}

// Adds a legend under a chart: one entry per [swatch class, text].
function showLegend(container, entries) {
  const list = htmlEl(container, "ul", "legend");
  for (const [swatch, text] of entries) {
    const item = htmlEl(list, "li", "");
    htmlEl(item, "span", `swatch ${swatch}`);
    item.append(text);
  }
}

// Formats a value range for a tooltip, closed on the right only for the last
// bin, which holds the maximum.
function rangeText(lo, hi, last) {
  return `[${formatValue(lo)}, ${formatValue(hi)}${last ? "]" : ")"}`;
}

// Returns "1 value" or "n values" with the given noun.
function plural(n, noun) {
  return `${n} ${noun}${n === 1 ? "" : "s"}`;
}

// ===== Section 4: Distribution ============================================

// Returns why a chart of the values' distribution (histogram, ECDF, Q-Q) has
// no payload; needsSD adds the Q-Q plot's zero-SD condition.
function shapeChartReason(s, needsSD) {
  if (s.count < 2) {
    return "it needs at least 2 values";
  }
  if (s.min === s.max) {
    return "every value is the same";
  }
  if (needsSD && !(s.stdDev > 0)) {
    return "the standard deviation is 0";
  }
  return "the range of the values is too large to draw";
}

// Returns why an input-order chart (trendline, lag-1) has no payload:
// suppressed under -T, or too few values (minCount), no spread or zero SD.
function orderChartReason(s, minCount) {
  if (s.trimDatasetPct > 0) {
    return "-T sorts the data, so order-based charts are meaningless";
  }
  if (s.count < minCount) {
    return `it needs at least ${minCount} values`;
  }
  if (s.min === s.max) {
    return "every value is the same";
  }
  if (!(s.stdDev > 0)) {
    return "the standard deviation is 0";
  }
  return "the range of the values is too large to draw";
}

// Shows the text output's sort-order warning on an input-order chart when the
// input arrived ascending or descending.
function showSortWarning(el, data) {
  const warning = data.distribution.sortWarning;
  if (warning) {
    showNote(el, `Input order is ${data.stats.inputOrder}: ${warning}.`, true);
  }
}

// Returns the mark of a center (mean, median) by key.
function centerMark(data, key) {
  return data.summary.center.marks.find((m) => m.key === key);
}

// The histogram's bin count chosen on the slider, kept across redraws after
// a resize; 0 until the first draw picks Go's default.
let histogramBins = 0;

// Draws the histogram with its bin-count slider. The slider steps by index
// through the offered counts (divisors of 960 no larger than n, so every
// count merges the fine bins exactly) and starts at Go's default.
function drawHistogram({ el, data, width, motion }) {
  const h = data.charts.histogram;
  if (!h) {
    showReason(el, shapeChartReason(data.stats, false));
    return;
  }
  if (!h.offered.includes(histogramBins)) {
    histogramBins = h.default;
  }
  const controls = htmlEl(el, "div", "hist-controls");
  const label = htmlEl(controls, "label", "hist-label", "Bins");
  const input = document.createElement("input");
  input.type = "range";
  input.min = "0";
  input.max = String(h.offered.length - 1);
  input.step = "1";
  input.value = String(h.offered.indexOf(histogramBins));
  label.appendChild(input);
  const output = htmlEl(controls, "output", "hist-count");
  const plot = htmlEl(el, "div", "");

  const render = (renderMotion) => {
    const text = `${plural(histogramBins, "bin")}${histogramBins === h.default ? " (default)" : ""}`;
    output.textContent = text;
    input.setAttribute("aria-valuetext", text);
    plot.replaceChildren();
    drawHistogramBins(plot, data, histogramBins, width, renderMotion);
  };
  input.addEventListener("input", () => {
    histogramBins = h.offered[Number(input.value)];
    hideTooltip();
    render(createMotion(false));
  });
  render(motion);
}

// Merges the 960 fine bins into count bars (count divides 960).
function mergeBins(fine, count) {
  const group = fine.length / count;
  const bars = new Array(count).fill(0);
  fine.forEach((c, i) => {
    bars[Math.floor(i / group)] += c;
  });
  return bars;
}

// Draws the histogram's bars for count bins, and the normal curve with the
// data's mean and SD scaled to the bars: n times the bin width times the
// normal density. The curve is omitted when the SD is 0, and clipped when its
// peak would squash the bars.
function drawHistogramBins(el, data, count, width, motion) {
  const h = data.charts.histogram;
  const s = data.stats;
  const bars = mergeBins(h.counts, count);
  const binWidth = (h.max - h.min) / count;
  const barMax = Math.max(...bars);
  const sd = s.stdDev;
  const scaleFactor = s.count * binWidth / (sd * Math.sqrt(2 * Math.PI));
  const curveAt = (v) => scaleFactor * Math.exp(-0.5 * ((v - s.mean) / sd) ** 2);
  const peak = sd > 0 ? curveAt(clamp(s.mean, h.min, h.max)) : 0;
  const showCurve = sd > 0 && Number.isFinite(peak);
  const yMax = Math.max(barMax, showCurve ? Math.min(peak, barMax * 1.5) : 0);

  const svg = newSvg(el, width);
  const f = plotFrame(svg, width, [h.min, h.max], [0, yMax * 1.05], { minStep: 1, title: "count" });
  const barLayer = svgEl(svg, "g");
  bars.forEach((c, i) => {
    const x0 = f.x(h.min + i * binWidth);
    const x1 = i === count - 1 ? f.right : f.x(h.min + (i + 1) * binWidth);
    const gap = x1 - x0 > 4 ? 1 : 0;
    if (c > 0) {
      svgEl(barLayer, "rect", { class: "bar", x: x0 + gap / 2, y: f.y(c), width: Math.max(0.5, x1 - x0 - gap), height: f.bottom - f.y(c) });
    }
  });
  growUp(motion, barLayer, f.left, f.bottom);

  if (showCurve) {
    const points = [];
    for (let px = f.left; px <= f.right + 1; px += 2) {
      const v = h.min + ((Math.min(px, f.right) - f.left) / (f.right - f.left)) * (h.max - h.min);
      points.push(`${Math.min(px, f.right)},${Math.max(f.y(curveAt(v)), f.top - 50)}`);
    }
    const curve = svgEl(svg, "polyline", { class: "normal-curve", points: points.join(" "), "clip-path": clipToPlot(svg, f) });
    fadeIn(motion, curve, 450);
  }

  addPlotTooltip(svg, f, (px) => {
    const i = clamp(Math.floor(((px - f.left) / (f.right - f.left)) * count), 0, count - 1);
    const lo = h.min + i * binWidth;
    const hi = i === count - 1 ? h.max : lo + binWidth;
    return [valueText(data, rangeText(lo, hi, i === count - 1)), plural(bars[i], "value")];
  });
  finishSvg(svg, width, finishPlot(el, svg, f, width, valueName(data)) + 2);

  if (!showCurve) {
    showNote(el, "Normal curve not shown: the standard deviation is 0.");
    return;
  }
  const clipped = peak > yMax * 1.05 ? "; its peak is above the top of the chart" : "";
  showNote(el, `Curve: the normal distribution with this data's mean (${centerMark(data, "mean").text}) and SD (${data.distribution.stdDev.text})${data.stats.logTransformed ? ` of ${valueName(data)}` : ""}, scaled to the bars${clipped}.`);
}

// Draws the ECDF as a step line (every value up to 1,000) or a line through
// the 1,001 quantiles at 0.1% steps, with Q1, the median, Q3, p95, p99 and
// any -p percentiles marked at (value, p).
function drawECDF({ el, data, width, motion }) {
  const e = data.charts.ecdf;
  if (!e) {
    showReason(el, shapeChartReason(data.stats, false));
    return;
  }
  const v = e.values;
  const svg = newSvg(el, width);
  const f = plotFrame(svg, width, padDomain(v[0], v[v.length - 1], 0.02), [0, 1], {
    format: (p) => `${Math.round(p * 100)}%`,
    title: "share of values at or below",
  });
  let d = `M${f.left},${f.y(0)}`;
  if (e.mode === "exact") {
    v.forEach((value, i) => {
      d += `H${f.x(value)}V${f.y((i + 1) / v.length)}`;
    });
  } else {
    v.forEach((value, i) => {
      d += `${i === 0 ? "H" + f.x(value) : ""}L${f.x(value)},${f.y(i / (v.length - 1))}`;
    });
  }
  d += `H${f.right}`;
  const line = svgEl(svg, "path", { class: "ecdf-line", d });
  revealLeftToRight(motion, line);

  // Marks: the ladder's percentiles plus the median, unless -p 50 gave it.
  const marks = data.summary.ladder.map((m) => ({ label: m.label, text: m.text, value: m.value, p: m.p }));
  if (!marks.some((m) => m.p === 50)) {
    const median = centerMark(data, "median");
    marks.push({ label: "Median", text: median.text, value: median.value, p: 50 });
  }
  marks.sort((a, b) => b.p - a.p);
  const markLayer = svgEl(svg, "g");
  const leaders = svgEl(markLayer, "g");
  const placed = []; // label boxes already placed, as [left, top, right, bottom]
  const clashes = (x0, baseline, w) => placed.some(([l, t, r, b]) => x0 < r + 3 && x0 + w > l - 3 && baseline - 10 < b && baseline + 1 > t);
  // In exact mode a mark sits on the step at its value: percentiles
  // interpolate between data values, so at small n the share of values at or
  // below p99 can be far from 99% (2 of 3 at n = 3). The quantile mode's
  // 0.1% steps put it within 0.1% of p.
  const exact = e.mode !== "quantiles";
  for (const m of marks) {
    const atOrBelow = exact ? countAtOrBelow(v, m.value) : 0;
    const share = exact ? atOrBelow / v.length : m.p / 100;
    const cx = f.x(m.value);
    const cy = f.y(share);
    const label = svgText(markLayer, 0, 0, m.label, { class: "halo" });
    const w = textWidth(label);
    // A rising curve leaves the space above-left and below-right of a point
    // empty: try above-left, then below-right, then push down from above-left.
    const left = Math.max(f.left + 2, cx - 4 - w);
    let [lx, ly] = [left, cy - 6];
    if (clashes(lx, ly, w)) {
      [lx, ly] = [Math.min(cx + 6, width - w), cy + 15];
      if (clashes(lx, ly, w)) {
        [lx, ly] = [left, cy - 6];
        while (clashes(lx, ly, w)) {
          ly += 11;
        }
      }
    }
    label.setAttribute("x", lx);
    label.setAttribute("y", ly);
    placed.push([lx, ly - 10, lx + w, ly + 1]);
    if (ly - cy > 20 || cy - ly > 14) {
      svgEl(leaders, "line", { class: "leader", x1: cx, y1: cy, x2: clamp(cx, lx, lx + w), y2: ly < cy ? ly + 2 : ly - 10 });
    }
    const dot = svgEl(markLayer, "circle", { class: "mark-dot", cx, cy, r: 3.5 });
    const shareText = exact ? `${atOrBelow} of ${v.length} values lie at or below it` : `${formatValue(m.p)}% of the values lie at or below it`;
    addTooltip(dot, [m.label, valueText(data, m.text), shareText]);
  }
  fadeIn(motion, markLayer, 900);
  finishSvg(svg, width, finishPlot(el, svg, f, width, valueName(data)) + 2);
  if (e.mode === "quantiles") {
    showNote(el, `Drawn through ${v.length} quantiles at 0.1% steps of the ${data.stats.count} values.`);
  }
}

// Returns how many of the ascending values are at or below x.
function countAtOrBelow(sorted, x) {
  let lo = 0;
  let hi = sorted.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid] <= x) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  return lo;
}

// Draws the normal Q-Q plot: each order statistic against the standard
// normal quantile of its plotting position, with the reference line through
// (Q1, z of 0.25) and (Q3, z of 0.75), which outliers do not tilt the way a
// mean and SD line would. Values run along the horizontal axis like every
// other value axis in the report.
function drawQQ({ el, data, width, motion }) {
  const q = data.charts.qq;
  if (!q) {
    showReason(el, shapeChartReason(data.stats, true));
    return;
  }
  const svg = newSvg(el, width);
  const f = plotFrame(svg, width, padDomain(q.x[0], q.x[q.x.length - 1], 0.03), padDomain(q.z[0], q.z[q.z.length - 1], 0.06), { title: "normal quantile (z)" });
  const [d0, d1] = f.x.domain;
  const ref = q.q3 > q.q1
    ? { x1: f.x(d0), y1: f.y(q.zq1 + ((d0 - q.q1) * (q.zq3 - q.zq1)) / (q.q3 - q.q1)), x2: f.x(d1), y2: f.y(q.zq1 + ((d1 - q.q1) * (q.zq3 - q.zq1)) / (q.q3 - q.q1)) }
    : { x1: f.x(q.q1), y1: f.bottom, x2: f.x(q.q1), y2: f.top };
  const refLine = svgEl(svg, "line", Object.assign({ class: "ref-line", "clip-path": clipToPlot(svg, f) }, ref));
  fadeIn(motion, refLine, 200);

  const xs = q.x.map(f.x);
  const ys = q.z.map(f.y);
  const r = xs.length > 300 ? 2 : 3;
  const dots = svgEl(svg, "g");
  xs.forEach((cx, i) => svgEl(dots, "circle", { class: "dot qq-dot", cx, cy: ys[i], r }));
  fadeIn(motion, dots, 400);
  const anchors = [["Q1", q.q1, q.zq1], ["Q3", q.q3, q.zq3]];
  for (const [, value, z] of anchors) {
    const anchor = svgEl(svg, "rect", { class: "anchor", x: f.x(value) - 4, y: f.y(z) - 4, width: 8, height: 8 });
    fadeIn(motion, anchor, 600);
  }
  addPlotTooltip(svg, f, (px, py) => {
    const a = anchors.find(([, value, z]) => Math.abs(f.x(value) - px) <= 5 && Math.abs(f.y(z) - py) <= 5);
    if (a) {
      return [`${a[0]}, a point of the reference line`, `${valueText(data, formatValue(a[1]), "value")}, z = ${minusSign(a[2].toFixed(4))}`];
    }
    const i = nearestPoint(xs, ys, px, py);
    return i < 0 ? null : [valueText(data, formatValue(q.x[i]), "Value"), `normal quantile z = ${minusSign(q.z[i].toFixed(3))}`];
  });
  finishSvg(svg, width, finishPlot(el, svg, f, width, valueName(data)) + 2);
  showNote(el, "Points along the line mean the values follow a normal distribution; the line runs through Q1 and Q3 (squares).");
  if (q.x.length < data.stats.count) {
    showNote(el, `Showing ${q.x.length} of the ${data.stats.count} sorted values, spaced evenly in z so both tails are included.`);
  }
}

// Returns the [start, end) input indices of trendline bucket i when n values
// are split into buckets chunks, the same boundaries Go uses.
function trendBounds(i, n, buckets) {
  const step = n / buckets;
  const start = Math.round(i * step);
  let end = Math.min(Math.round((i + 1) * step), n);
  if (end <= start) {
    end = start + 1;
  }
  return [start, end];
}

// Draws the trendline: the values in input order (or, above 1,000 values,
// each bucket's mean with its min-to-max band), the mean line and, under -e,
// the EMA line, all uncovered left to right on scroll-in.
function drawTrendline({ el, data, width, motion }) {
  const t = data.charts.trendline;
  const s = data.stats;
  if (!t) {
    showReason(el, orderChartReason(s, 2));
    return;
  }
  const n = s.count;
  const raw = t.mode === "raw";
  const points = []; // one per raw value or bucket: position, value lines
  if (raw) {
    t.values.forEach((value, i) => points.push({ pos: i + 1, y: value, emaPos: i + 1, ema: t.ema && t.ema[i], lines: [`Position ${i + 1}`, valueText(data, formatValue(value), "value")] }));
  } else {
    t.mean.forEach((mean, i) => {
      const [start, end] = trendBounds(i, n, t.mean.length);
      points.push({
        pos: (start + end + 1) / 2,
        y: mean,
        emaPos: end,
        ema: t.ema && t.ema[i],
        lo: t.min[i],
        hi: t.max[i],
        lines: [`Positions ${start + 1} to ${end}`, `mean ${valueText(data, formatValue(mean))}`, `min ${valueText(data, formatValue(t.min[i]))}, max ${valueText(data, formatValue(t.max[i]))}`],
      });
    });
  }
  const svg = newSvg(el, width);
  const f = plotFrame(svg, width, [1, Math.max(2, n)], padDomain(s.min, s.max, 0.05), { title: valueName(data) });
  const reveal = svgEl(svg, "g");
  if (!raw) {
    const upper = points.map((p) => `${f.x(p.pos)},${f.y(p.hi)}`);
    const lower = points.map((p) => `${f.x(p.pos)},${f.y(p.lo)}`).reverse();
    svgEl(reveal, "polygon", { class: "trend-band", points: upper.concat(lower).join(" ") });
  }
  const mean = centerMark(data, "mean");
  const my = f.y(mean.value);
  svgEl(reveal, "line", { class: "mean-line", x1: f.left, x2: f.right, y1: my, y2: my });
  svgEl(reveal, "polyline", { class: "trend-line", points: points.map((p) => `${f.x(p.pos)},${f.y(p.y)}`).join(" ") });
  if (t.ema) {
    svgEl(reveal, "polyline", { class: "ema-line", points: points.map((p) => `${f.x(p.emaPos)},${f.y(p.ema)}`).join(" ") });
  }
  revealLeftToRight(motion, reveal);
  // The label is left out when the mean's text is wider than the plot.
  const meanLabel = svgText(svg, f.right - 2, my - 5, `Mean ${mean.text}`, { class: "halo", "text-anchor": "end" });
  if (textWidth(meanLabel) > f.right - f.left - 4) {
    meanLabel.remove();
  } else {
    fadeIn(motion, meanLabel, 1200);
  }

  const xs = points.map((p) => f.x(p.pos));
  addPlotTooltip(svg, f, (px) => {
    let best = 0;
    xs.forEach((x, i) => {
      if (Math.abs(x - px) < Math.abs(xs[best] - px)) {
        best = i;
      }
    });
    const p = points[best];
    return t.ema ? p.lines.concat(`EMA ${valueText(data, formatValue(p.ema))} at position ${p.emaPos}`) : p.lines;
  });
  finishSvg(svg, width, finishPlot(el, svg, f, width, "input position", 1) + 2);

  const legend = [raw ? ["sw-line", "Values"] : ["sw-line", "Bucket mean"]];
  if (!raw) {
    legend.push(["sw-band", "Bucket min to max"]);
  }
  legend.push(["sw-mean", "Mean"]);
  if (t.ema) {
    legend.push(["sw-ema", `${data.distribution.ema.label}, ending at ${data.distribution.ema.text}${lnSuffix(data)}`]);
  }
  showLegend(el, legend);
  if (!raw) {
    showNote(el, `The ${n} values are split into ${t.mean.length} buckets of consecutive positions.`);
  }
  showSortWarning(el, data);
}

// Sizes of the lag-1 plot.
const LAG_MAX_SIDE = 340; // largest side of the square plot
const LAG_SWATCHES = 5; // steps shown in the density grid's legend
const LAG_MIN_SHADE = 0.12; // opacity floor of the shade scale, approached as a count goes to 0

// Returns the opacity of a density-grid cell holding count pairs on a
// square-root scale up to maxCount.
function lagShade(count, maxCount) {
  return LAG_MIN_SHADE + (1 - LAG_MIN_SHADE) * Math.sqrt(count / maxCount);
}

// Returns the counts the density grid's legend shows: 1, then counts evenly
// spaced in shade up to maxCount, without repeats, so every swatch is the
// exact shade of a cell holding that count.
function lagLegendCounts(maxCount) {
  const counts = [1];
  for (let k = 1; k < LAG_SWATCHES; k++) {
    const t = k / (LAG_SWATCHES - 1);
    const c = Math.round(maxCount * t * t);
    if (c > counts[counts.length - 1]) {
      counts.push(c);
    }
  }
  return counts;
}

// Draws the lag-1 plot: each value against the next, as dots up to 5,000
// pairs, otherwise as a 64 x 64 grid of pair counts shaded on a square-root
// scale (a linear one at 1M values lights only the central cells), with a
// legend. A dashed diagonal marks next = current.
function drawLag({ el, data, width, motion }) {
  const l = data.charts.lag;
  const s = data.stats;
  if (!l) {
    showReason(el, orderChartReason(s, 3));
    return;
  }
  const grid = l.mode === "grid" ? l.grid : null;
  const domain = grid ? [grid.min, grid.max] : padDomain(s.min, s.max, 0.03);
  const svg = newSvg(el, width);
  const f = plotFrame(svg, width, domain, domain, { title: `next ${valueName(data)}`, square: true, height: clamp(width - 80, 160, LAG_MAX_SIDE) });
  svgEl(svg, "line", { class: "leader", x1: f.x(domain[0]), y1: f.y(domain[0]), x2: f.x(domain[1]), y2: f.y(domain[1]) });
  const layer = svgEl(svg, "g");
  let pick;
  if (grid) {
    const size = grid.size;
    const cell = (grid.max - grid.min) / size;
    const maxCount = Math.max(...grid.counts);
    grid.counts.forEach((c, i) => {
      if (c > 0) {
        const col = i % size;
        const row = Math.floor(i / size);
        const x0 = f.x(grid.min + col * cell);
        const y0 = f.y(grid.min + (row + 1) * cell);
        svgEl(layer, "rect", { class: "cell", x: x0, y: y0, width: f.x(grid.min + (col + 1) * cell) - x0 + 0.3, height: f.y(grid.min + row * cell) - y0 + 0.3, "fill-opacity": lagShade(c, maxCount).toFixed(3) });
      }
    });
    pick = (px, py) => {
      const col = clamp(Math.floor(((px - f.left) / (f.right - f.left)) * size), 0, size - 1);
      const row = clamp(Math.floor(((f.bottom - py) / (f.bottom - f.top)) * size), 0, size - 1);
      const c = grid.counts[row * size + col];
      return [plural(c, "pair"), `${valueName(data)} in ${rangeText(grid.min + col * cell, grid.min + (col + 1) * cell, col === size - 1)}`, `next ${valueText(data, `in ${rangeText(grid.min + row * cell, grid.min + (row + 1) * cell, row === size - 1)}`)}`];
    };
  } else {
    const v = l.values;
    const xs = v.slice(0, -1).map(f.x);
    const ys = v.slice(1).map(f.y);
    // Fewer pairs get larger, more opaque dots; many overlap, so they stay faint.
    const [r, opacity] = xs.length > 1000 ? [1.8, 0.3] : xs.length > 200 ? [2.5, 0.45] : [3.5, 0.65];
    xs.forEach((cx, i) => svgEl(layer, "circle", { class: "dot lag-dot", cx, cy: ys[i], r, "fill-opacity": opacity }));
    pick = (px, py) => {
      const i = nearestPoint(xs, ys, px, py);
      return i < 0 ? null : [`Positions ${i + 1} and ${i + 2}`, valueText(data, formatValue(v[i]), "value"), `next ${valueText(data, formatValue(v[i + 1]))}`];
    };
  }
  fadeIn(motion, layer, 200);
  addPlotTooltip(svg, f, pick);
  let bottom = finishPlot(el, svg, f, width, valueName(data));

  if (grid) {
    // Legend: swatches for real counts, each in the shade a cell with that
    // count gets. The entries wrap onto another line at the right edge.
    const maxCount = Math.max(...grid.counts);
    const legend = svgEl(svg, "g");
    const title = svgText(legend, 0, bottom + 16, "pairs per cell", { class: "muted" });
    let lx = textWidth(title) + 10;
    let ly = bottom + 16;
    for (const count of lagLegendCounts(maxCount)) {
      const label = svgText(legend, 0, 0, String(count));
      const entryWidth = 18 + textWidth(label);
      if (lx + entryWidth > width) {
        lx = 0;
        ly += 18;
      }
      svgEl(legend, "rect", { class: "cell", x: lx, y: ly - 10, width: 14, height: 12, "fill-opacity": lagShade(count, maxCount).toFixed(3) });
      label.setAttribute("x", lx + 18);
      label.setAttribute("y", ly);
      lx += entryWidth + 10;
    }
    bottom = ly + 8;
  }
  finishSvg(svg, width, bottom + 2);

  const ac = data.distribution.autocorrelation;
  if (ac) {
    showNote(el, `Autocorrelation ${ac.text} (${ac.reading}).`);
  }
  if (grid) {
    showNote(el, `${s.count - 1} pairs are too many to draw as dots, so they are counted in a ${grid.size} × ${grid.size} grid; shading uses a square-root scale so sparse cells stay visible.`);
  }
  showSortWarning(el, data);
}

// ===== Startup ============================================================

// The drawing function for each data-chart name in the template.
const CHART_DRAWERS = {
  distinct: drawDistinctBar,
  range: drawRangeStrip,
  center: drawCenterLine,
  box: drawBoxPlot,
  ladder: drawLadder,
  skewness: drawGauge,
  kurtosis: drawGauge,
  cv: drawGauge,
  histogram: drawHistogram,
  ecdf: drawECDF,
  qq: drawQQ,
  trendline: drawTrendline,
  lag: drawLag,
};

const reportData = loadReportData();
useMinusSigns(reportData.summary);
useMinusSigns(reportData.distribution);
initDefinitions();
mountCharts(reportData, CHART_DRAWERS);
