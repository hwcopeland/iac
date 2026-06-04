// Builds the periodic-table grid. Layout uses the dataset's xpos/ypos with the
// standard separated f-block (lanthanides on ypos 9, actinides on ypos 10).
// A leading column carries period numbers and a header row carries group
// numbers, so the grid reads like a printed table.

import type { Element } from "../types";
import { h } from "./dom";

export interface BuiltGrid {
  grid: HTMLElement;
  cells: Map<number, HTMLElement>;
}

function tile(e: Element, onSelect: (e: Element) => void): HTMLElement {
  const cell = h(
    "button",
    {
      class: "tile",
      type: "button",
      "data-z": e.number,
      "aria-label": `${e.number} ${e.name}`,
      title: `${e.name} — ${e.categoryRaw}`,
      style: `grid-column:${e.xpos + 1};grid-row:${e.ypos + 1};`,
      onclick: () => onSelect(e),
    },
    h("span", { class: "z" }, String(e.number)),
    h("span", { class: "sym" }, e.symbol),
    h("span", { class: "tname" }, e.name),
    h("span", { class: "mass" }, e.atomicMass ? e.atomicMass.toFixed(2) : ""),
  );
  return cell;
}

function placeholder(label: string, col: number, rowYpos: number): HTMLElement {
  return h(
    "div",
    {
      class: "tile placeholder",
      style: `grid-column:${col + 1};grid-row:${rowYpos + 1};`,
      "aria-hidden": "true",
    },
    h("span", { class: "sym small" }, label),
  );
}

export function buildGrid(elements: Element[], onSelect: (e: Element) => void): BuiltGrid {
  const grid = h("div", { class: "ptable", role: "grid", "aria-label": "Periodic table" });
  const cells = new Map<number, HTMLElement>();

  // group-number header (row 1, columns 2..19)
  for (let g = 1; g <= 18; g++) {
    grid.appendChild(
      h("div", { class: "axis group", style: `grid-column:${g + 1};grid-row:1;` }, String(g)),
    );
  }
  // period-number labels (column 1, rows 2..8 → periods 1..7)
  for (let p = 1; p <= 7; p++) {
    grid.appendChild(
      h("div", { class: "axis period", style: `grid-column:1;grid-row:${p + 1};` }, String(p)),
    );
  }

  // f-block placeholders inside the main table (group 3, periods 6 & 7)
  grid.appendChild(placeholder("57–71", 3, 6));
  grid.appendChild(placeholder("89–103", 3, 7));

  for (const e of elements) {
    const t = tile(e, onSelect);
    cells.set(e.number, t);
    grid.appendChild(t);
  }
  return { grid, cells };
}
