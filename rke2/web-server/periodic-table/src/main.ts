// App entry: wires the grid, trend selector, legend, search, detail drawer and
// keyboard navigation together. State is intentionally tiny and re-rendered
// imperatively — there is no framework.

import "./style.css";
import { ELEMENTS, searchElements } from "./data/elements";
import type { Element, TrendKey } from "./types";
import {
  CATEGORY_COLORS,
  MISSING_COLOR,
  TRENDS,
  colorFor,
  numericRange,
  trendMeta,
} from "./trends";
import { fmtNum } from "./format";
import { buildDetail } from "./ui/detail";
import { buildGrid } from "./ui/grid";
import { clear, h } from "./ui/dom";

const root = document.getElementById("app")!;

let activeTrend: TrendKey = "category";
let selected: Element | null = null;
let dimmed = new Set<number>(); // elements faded out by an active search

const { grid, cells } = buildGrid(ELEMENTS, select);
const posIndex = new Map<string, Element>();
for (const e of ELEMENTS) posIndex.set(`${e.xpos},${e.ypos}`, e);

// ---- header --------------------------------------------------------------

const search = h("input", {
  class: "search",
  type: "search",
  placeholder: "Search name, symbol, or number…  ( / )",
  "aria-label": "Search elements",
  oninput: onSearch,
}) as HTMLInputElement;

const trendButtons = h("div", { class: "trend-buttons", role: "tablist" });
for (const t of TRENDS) {
  trendButtons.appendChild(
    h(
      "button",
      {
        class: "trend-btn",
        type: "button",
        role: "tab",
        "data-trend": t.key,
        onclick: () => setTrend(t.key),
      },
      t.label,
    ),
  );
}

const legend = h("div", { class: "legend", "aria-live": "polite" });

const header = h(
  "header",
  { class: "app-header" },
  h(
    "div",
    { class: "title-row" },
    h("h1", {}, "Dynamic Periodic Table"),
    h("p", { class: "subtitle" }, "graduate-level reference · 118 elements"),
  ),
  search,
  h("div", { class: "color-by" }, h("span", { class: "color-by-label" }, "Color by:"), trendButtons),
  legend,
);

// ---- detail drawer -------------------------------------------------------

const drawerBody = h("div", { class: "drawer-body" });
const drawer = h(
  "aside",
  { class: "drawer", "aria-label": "Element details", "aria-hidden": "true" },
  h(
    "div",
    { class: "drawer-bar" },
    h("button", { class: "drawer-close", type: "button", "aria-label": "Close", onclick: closeDrawer }, "✕"),
  ),
  drawerBody,
);

const main = h("main", { class: "app-main" }, h("div", { class: "table-scroll" }, grid));

root.appendChild(header);
root.appendChild(main);
root.appendChild(drawer);

// ---- rendering -----------------------------------------------------------

function renderTrend(): void {
  const meta = trendMeta(activeTrend);
  const range = numericRange(ELEMENTS, activeTrend);
  for (const e of ELEMENTS) {
    const cell = cells.get(e.number);
    if (!cell) continue;
    const color = colorFor(e, activeTrend, range);
    cell.style.setProperty("--tile", color);
    cell.classList.toggle("missing", color === MISSING_COLOR);
  }
  for (const btn of trendButtons.querySelectorAll<HTMLElement>(".trend-btn")) {
    btn.classList.toggle("active", btn.dataset.trend === activeTrend);
  }
  renderLegend(meta.kind, range);
}

function renderLegend(kind: "categorical" | "numeric", range: { min: number; max: number }): void {
  clear(legend);
  const meta = trendMeta(activeTrend);
  if (kind === "categorical") {
    const palette =
      activeTrend === "category"
        ? CATEGORY_COLORS
        : activeTrend === "phase"
          ? { Gas: "#74c0fc", Liquid: "#63e6be", Solid: "#ffa94d", Unknown: "#868e96" }
          : { s: "#ff8787", p: "#4dabf7", d: "#ffd43b", f: "#f783ac" };
    for (const [label, color] of Object.entries(palette)) {
      legend.appendChild(
        h("span", { class: "legend-item" }, h("span", { class: "swatch", style: `background:${color}` }), label),
      );
    }
  } else {
    const grad = h("div", { class: "legend-gradient" });
    legend.appendChild(h("span", { class: "legend-min" }, `${fmtNum(range.min, 2)}`));
    legend.appendChild(grad);
    legend.appendChild(h("span", { class: "legend-max" }, `${fmtNum(range.max, 2)}`));
    legend.appendChild(h("span", { class: "legend-unit" }, meta.unit ?? ""));
  }
}

function renderSelection(): void {
  for (const [z, cell] of cells) {
    cell.classList.toggle("selected", selected?.number === z);
  }
}

function renderDim(): void {
  for (const [z, cell] of cells) {
    cell.classList.toggle("dim", dimmed.size > 0 && !dimmed.has(z));
  }
}

// ---- actions -------------------------------------------------------------

function setTrend(key: TrendKey): void {
  activeTrend = key;
  renderTrend();
}

function select(e: Element): void {
  selected = e;
  clear(drawerBody);
  drawerBody.appendChild(buildDetail(e));
  drawer.classList.add("open");
  drawer.setAttribute("aria-hidden", "false");
  renderSelection();
  cells.get(e.number)?.focus();
}

function closeDrawer(): void {
  drawer.classList.remove("open");
  drawer.setAttribute("aria-hidden", "true");
  selected = null;
  renderSelection();
}

function onSearch(ev: Event): void {
  const q = (ev.target as HTMLInputElement).value;
  if (!q.trim()) {
    dimmed = new Set();
  } else {
    dimmed = new Set(searchElements(q).map((e) => e.number));
  }
  renderDim();
}

// ---- keyboard navigation -------------------------------------------------

function move(dx: number, dy: number): void {
  const cur = selected ?? ELEMENTS[0];
  // Walk in the requested direction until we hit an occupied cell or leave grid.
  for (let step = 1; step <= 32; step++) {
    const nx = cur.xpos + dx * step;
    const ny = cur.ypos + dy * step;
    if (nx < 1 || nx > 18 || ny < 1 || ny > 10) break;
    const found = posIndex.get(`${nx},${ny}`);
    if (found) {
      select(found);
      return;
    }
  }
}

window.addEventListener("keydown", (ev) => {
  if (ev.key === "/" && document.activeElement !== search) {
    ev.preventDefault();
    search.focus();
    return;
  }
  if (ev.key === "Escape") {
    if (document.activeElement === search) {
      search.blur();
    } else {
      closeDrawer();
    }
    return;
  }
  if (document.activeElement === search) return;
  switch (ev.key) {
    case "ArrowLeft":
      ev.preventDefault();
      move(-1, 0);
      break;
    case "ArrowRight":
      ev.preventDefault();
      move(1, 0);
      break;
    case "ArrowUp":
      ev.preventDefault();
      move(0, -1);
      break;
    case "ArrowDown":
      ev.preventDefault();
      move(0, 1);
      break;
  }
});

// ---- boot ----------------------------------------------------------------

renderTrend();
