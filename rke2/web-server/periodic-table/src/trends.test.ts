import { describe, expect, it } from "vitest";
import { ELEMENTS, BY_SYMBOL } from "./data/elements";
import {
  CATEGORY_COLORS,
  MISSING_COLOR,
  colorFor,
  maxAnionicState,
  numericRange,
  numericValue,
  scaleColor,
} from "./trends";

describe("scaleColor", () => {
  it("anchors the endpoints to the first and last scale stops", () => {
    expect(scaleColor(0)).toBe("rgb(13, 27, 42)");
    expect(scaleColor(1)).toBe("rgb(231, 76, 60)");
  });
  it("clamps out-of-range t", () => {
    expect(scaleColor(-5)).toBe(scaleColor(0));
    expect(scaleColor(5)).toBe(scaleColor(1));
  });
  it("produces a distinct mid color", () => {
    expect(scaleColor(0.5)).not.toBe(scaleColor(0));
    expect(scaleColor(0.5)).not.toBe(scaleColor(1));
  });
});

describe("numericValue / numericRange", () => {
  it("pulls the first ionization energy", () => {
    const h = BY_SYMBOL.get("h")!;
    expect(numericValue(h, "firstIonizationEnergy")).toBeCloseTo(h.ionizationEnergies[0]);
  });
  it("computes a finite range that spans the data", () => {
    const r = numericRange(ELEMENTS, "atomicMass");
    expect(r.min).toBeLessThan(r.max);
    expect(r.min).toBeGreaterThan(0);
    expect(Number.isFinite(r.max)).toBe(true);
  });
});

describe("maxAnionicState / anionicStability", () => {
  it("returns the deepest negative oxidation state", () => {
    expect(maxAnionicState(BY_SYMBOL.get("c")!)).toBe(-4); // carbide
    expect(maxAnionicState(BY_SYMBOL.get("o")!)).toBe(-2); // oxide
    expect(maxAnionicState(BY_SYMBOL.get("f")!)).toBe(-1); // fluoride
  });
  it("is 0 for elements that form no anion", () => {
    expect(maxAnionicState(BY_SYMBOL.get("na")!)).toBe(0);
    expect(maxAnionicState(BY_SYMBOL.get("fe")!)).toBe(0);
  });
  it("is null where the chemistry is unknown", () => {
    expect(maxAnionicState(BY_SYMBOL.get("mt")!)).toBeNull();
  });
  it("colors the trend by anion depth (magnitude)", () => {
    expect(numericValue(BY_SYMBOL.get("c")!, "anionicStability")).toBe(4);
    expect(numericValue(BY_SYMBOL.get("na")!, "anionicStability")).toBe(0);
    expect(numericValue(BY_SYMBOL.get("mt")!, "anionicStability")).toBeNull();
  });
});

describe("colorFor", () => {
  const range = numericRange(ELEMENTS, "electronegativity");
  it("uses the category palette for the categorical trend", () => {
    const na = BY_SYMBOL.get("na")!;
    expect(colorFor(na, "category", range)).toBe(CATEGORY_COLORS["alkali metal"]);
  });
  it("greys elements with missing numeric data", () => {
    const og = BY_SYMBOL.get("og")!; // no Pauling electronegativity
    expect(colorFor(og, "electronegativity", range)).toBe(MISSING_COLOR);
  });
  it("maps the most electronegative element near the top of the scale", () => {
    const f = BY_SYMBOL.get("f")!;
    expect(colorFor(f, "electronegativity", range)).toBe(scaleColor(1));
  });
});
