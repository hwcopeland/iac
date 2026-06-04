import { describe, expect, it, vi } from "vitest";
import { BY_SYMBOL, ELEMENTS } from "../data/elements";
import { buildGrid } from "./grid";
import { buildDetail } from "./detail";

describe("buildGrid", () => {
  it("renders a clickable tile for every element", () => {
    const { grid, cells } = buildGrid(ELEMENTS, () => {});
    expect(cells.size).toBe(118);
    expect(grid.querySelectorAll("button.tile").length).toBe(118);
  });

  it("invokes the select callback with the element when a tile is clicked", () => {
    const onSelect = vi.fn();
    const { cells } = buildGrid(ELEMENTS, onSelect);
    cells.get(26)!.click();
    expect(onSelect).toHaveBeenCalledOnce();
    expect(onSelect.mock.calls[0][0].symbol).toBe("Fe");
  });

  it("positions tiles via CSS grid coordinates", () => {
    const { cells } = buildGrid(ELEMENTS, () => {});
    const h = cells.get(1)!;
    expect(h.style.gridColumn).toBe("2"); // xpos 1 + 1 (period-label column)
    expect(h.style.gridRow).toBe("2"); // ypos 1 + 1 (group-header row)
  });
});

describe("buildDetail", () => {
  it("renders electron configuration, a shell diagram and an IE chart", () => {
    const fe = BY_SYMBOL.get("fe")!;
    const node = buildDetail(fe);
    expect(node.querySelector(".detail-symbol")!.textContent).toBe("Fe");
    expect(node.querySelector("svg.shell-diagram")).not.toBeNull();
    expect(node.querySelector("svg.ie-chart rect")).not.toBeNull();
    expect(node.querySelectorAll(".ox").length).toBeGreaterThan(0);
  });

  it("handles superheavy elements with no oxidation data gracefully", () => {
    const og = BY_SYMBOL.get("og")!;
    const node = buildDetail(og);
    expect(node.textContent).toContain("predicted / unknown");
  });
});
