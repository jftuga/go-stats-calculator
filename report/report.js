"use strict";

// report.js draws the stats report's interactive parts from the JSON data
// block Go embeds in the page: definition popovers, and charts built as SVG
// with createElementNS and textContent only (no markup is ever parsed from a
// string), so no value or file name can be interpreted as markup. Sections, in order: shared helpers,
// definitions, the chart kit (SVG, scales, axes, labels, tooltip, animation,
// mounting), the charts of report sections 1 to 3, and startup.

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

// Shows a one-line reason in place of a chart that has nothing to draw.
function showReason(container, text) {
  htmlEl(container, "p", "chart-reason", text);
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
// power of ten. Values are i * step so they do not accumulate rounding error.
function niceTicks(lo, hi, count) {
  const raw = (hi - lo) / Math.max(1, count);
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const norm = raw / magnitude;
  const step = (norm < 1.5 ? 1 : norm < 3 ? 2 : norm < 7 ? 5 : 10) * magnitude;
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

// Uses a true minus sign for generated labels.
function minusSign(text) {
  return text.replace("-", "−");
}

// Unit suffixes for large tick values, largest first.
const TICK_UNITS = [
  [1e12, "T"],
  [1e9, "B"],
  [1e6, "M"],
  [1e3, "k"],
];

// Formats an axis tick with just enough decimals for its step. Large values
// with a coarse step use a k, M, B or T suffix when that needs at most two
// decimals; otherwise every digit is kept, and drawAxis switches to offsets
// when the result is too long.
function formatTick(v, step, maxAbs) {
  if (v === 0) {
    return "0";
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
// up, one more per leading zero below that, trailing zeros trimmed. Used only
// for values Go does not send as text (fences, whiskers, bin centers).
function formatValue(v) {
  if (Number.isInteger(v)) {
    return v.toFixed(0);
  }
  const a = Math.abs(v);
  const decimals = a >= 1 ? 4 : 4 - Math.floor(Math.log10(a)) - 1;
  return trimZeros(v.toFixed(Math.min(decimals, 100)));
}

// Draws a horizontal axis at y for scale: a baseline, ticks and labels kept
// inside the SVG, with a note under the labels when they are offsets. Ticks
// are thinned to every other one until no two labels overlap. Returns the y
// just below the labels or the note.
function drawAxis(svg, scale, y, width) {
  const [r0, r1] = scale.range;
  const [d0, d1] = scale.domain;
  const g = svgEl(svg, "g", { class: "axis" });
  svgEl(g, "line", { x1: r0, x2: r1, y1: y, y2: y });
  let { values, step } = niceTicks(d0, d1, Math.max(2, Math.floor((r1 - r0) / TICK_SPACING)));
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
    svgText(g, r0, y + 29, `ticks are offsets from ${minusSign(formatValue(base))}`, { class: "muted" });
    return y + 34;
  }
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
    showReason(mount.el, "This chart could not be drawn.");
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
    showReason(el, "No range to draw: every value is the same.");
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
  addTooltip(box, ["Q1 to Q3", `[${m.q1.text}, ${m.q3.text}]`, "The middle half of the values"]);
  for (const mark of marks) {
    const mx = x(mark.value);
    const reach = mark.key === "median" ? 11 : 7;
    svgEl(strip, "line", { class: mark.key === "median" ? "median-line" : "strip-tick", x1: mx, x2: mx, y1: y - reach, y2: y + reach });
    addTooltip(svgEl(strip, "rect", { class: "hit", x: mx - 5, y: y - 12, width: 10, height: 24 }), [mark.label, mark.text]);
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
  finishSvg(svg, width, top + (lanes - 1) * LANE_HEIGHT + 6);
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
    showReason(el, `Nothing to compare: every center is ${c.marks[0].text}.`);
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
  addTooltip(shade, [q.label, q.text]);
  const clipped = x(q.low) < r0 || x(q.high) > r1;

  svgEl(lineLayer, "line", { class: "guide", x1: r0, x2: r1, y1: lineY, y2: lineY });
  let ciWidened = false;
  if (c.ci) {
    const mid = x((c.ci.low + c.ci.high) / 2);
    const trueWidth = x(c.ci.high) - x(c.ci.low);
    ciWidened = trueWidth < MIN_CI_WIDTH;
    const w = Math.max(trueWidth, MIN_CI_WIDTH);
    const band = svgEl(lineLayer, "rect", { class: "ci-band", x: mid - w / 2, y: lineY - 8, width: w, height: 16 });
    addTooltip(band, ["95% CI (mean)", c.ci.text, ciWidened ? "Drawn at a minimum width; the true band is narrower" : ""]);
    growIn(motion, band, mid, lineY);
  }
  if (c.modalBin) {
    const b = c.modalBin;
    const bx = x(b.low);
    const bin = svgEl(lineLayer, "rect", { class: "modal-bin", x: bx, y: lineY - 6, width: Math.max(2, x(b.high) - bx), height: 12 });
    const valueWord = b.count === 1 ? "value" : "values";
    addTooltip(bin, ["Modal bin", b.text, `${b.count} ${valueWord}, the densest of ${b.bins} equal-width bins (-b)`, "From the -b binning, so it need not match the histogram's tallest bar"]);
    fadeIn(motion, bin, 300);
  }

  // Markers slide out from the median into place.
  const medianX = x(c.marks.find((mark) => mark.key === "median").value);
  for (const mark of c.marks) {
    const mx = x(mark.value);
    const outer = svgEl(markLayer, "g", { transform: `translate(${mx}, ${lineY})` });
    const inner = svgEl(outer, "g");
    centerMarker(inner, mark.key);
    addTooltip(svgEl(inner, "circle", { class: "hit", r: 9 }), [mark.label, mark.text]);
    slideIn(motion, inner, medianX - mx);
  }

  const bottom = drawAxis(svg, x, axisY, width);
  finishSvg(svg, width, bottom + 2);

  showNote(el, `Shaded: Q1 to Q3, ${q.text}${clipped ? ", wider than this view" : ""}.`);
  if (ciWidened) {
    showNote(el, "The 95% CI is narrower than a pixel at this scale, so its band is drawn at a minimum width.");
  }
  if (c.modeCount > 1) {
    showNote(el, `Mode not drawn: the data has ${c.modeCount} modes, listed in the Mode row.`);
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
    showReason(el, s.min === s.max ? "No box plot: every value is the same." : "No box plot: no value lies inside the fences.");
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
    addTooltip(svgEl(svg, "rect", { class: "hit", x: fx - 4, y: y - 20, width: 8, height: 40 }), [name, formatValue(value), rule]);
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
    addTooltip(svgEl(box, "rect", { class: "hit", x: Math.min(from, to), y: y - 8, width: Math.max(8, Math.abs(to - from)), height: 16 }), [name, formatValue(value), `The ${where}`]);
  }
  const rect = svgEl(box, "rect", { class: "box", x: q1x, y: y - 14, width: Math.max(2, q3x - q1x), height: 28 });
  addTooltip(rect, ["Box: Q1 to Q3", `[${formatValue(b.q1)}, ${formatValue(b.q3)}]`, `Median ${formatValue(b.median)}`]);
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
    const valueWord = count === 1 ? "value" : "values";
    const lines = o.binned ? [`Outliers near ${formatValue(v)}`, `${count} ${valueWord} in this bin`] : ["Outlier", formatValue(v), count > 1 ? `${count} values` : ""];
    addTooltip(dot, lines);
  });
  fadeIn(motion, dots, 450);

  const bottom = drawAxis(svg, x, y + 28, width);
  finishSvg(svg, width, bottom + 2);

  showNote(el, `Whiskers end at the most extreme values inside the fences, ${k} × IQR beyond the quartiles: [${formatValue(b.lowerFence)}, ${formatValue(b.upperFence)}].`);
  const total = o.counts.reduce((a, c) => a + c, 0);
  if (total === 0) {
    showNote(el, "No IQR outliers.");
  } else if (o.binned) {
    showNote(el, `${total} outliers, binned into ${o.values.length} positions; dot size shows how many values each holds.`);
  } else {
    const repeats = maxCount > 1 ? "; larger dots are repeated values" : "";
    showNote(el, `${total} ${total === 1 ? "outlier" : "outliers"} drawn as dots${repeats}.`);
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
    showReason(el, `Nothing to compare: every percentile is ${rungs[0].text}.`);
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
    addTooltip(dot, [rung.label, rung.text]);
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

  const bottom = drawAxis(svg, x, top + rowHeight * rungs.length + 4, width);
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
  const unitText = (v) => minusSign(formatValue(v)) + spec.unit;

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
  const valueText = off ? `${g.text} (off scale)` : g.text;
  addTooltip(svgEl(inner, "rect", { class: "hit", x: -6, y: trackY - 8, width: 12, height: trackH + 12 }), [spec.title, valueText, g.reading]);
  slideIn(motion, inner, x(clamp(0, e0, e1)) - nx);

  const label = svgText(svg, nx, trackY - 8, valueText, { class: "halo", "text-anchor": "middle" });
  const half = textWidth(label) / 2;
  label.setAttribute("x", clamp(nx, half, width - half));
  fadeIn(motion, label, 500);

  if (g.warning) {
    showNote(el, `Warning: ${g.warning}, so CV may not be meaningful.`, true);
  }
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
};

const reportData = loadReportData();
initDefinitions();
mountCharts(reportData, CHART_DRAWERS);
