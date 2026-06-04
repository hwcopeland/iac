import { describe, expect, it } from "vitest";
import { BY_SYMBOL, ELEMENTS, searchElements } from "./elements";

describe("dataset integrity", () => {
  it("contains exactly 118 elements", () => {
    expect(ELEMENTS.length).toBe(118);
  });

  it("has contiguous atomic numbers 1..118 in order", () => {
    ELEMENTS.forEach((e, i) => expect(e.number).toBe(i + 1));
  });

  it("every element has a symbol, name and positive atomic mass", () => {
    for (const e of ELEMENTS) {
      expect(e.symbol).toMatch(/^[A-Z][a-z]?$/);
      expect(e.name.length).toBeGreaterThan(0);
      expect(e.atomicMass).toBeGreaterThan(0);
    }
  });

  it("places every element at a unique grid coordinate", () => {
    const seen = new Set<string>();
    for (const e of ELEMENTS) {
      const key = `${e.xpos},${e.ypos}`;
      expect(seen.has(key)).toBe(false);
      seen.add(key);
      expect(e.xpos).toBeGreaterThanOrEqual(1);
      expect(e.xpos).toBeLessThanOrEqual(18);
    }
  });

  it("electron shell occupancies sum to the atomic number", () => {
    for (const e of ELEMENTS) {
      const sum = e.shells.reduce((a, b) => a + b, 0);
      expect(sum, `${e.symbol} shells`).toBe(e.number);
    }
  });

  it("assigns a known category to every element", () => {
    for (const e of ELEMENTS) {
      expect(e.category, e.symbol).not.toBe("unknown");
    }
  });

  it("derives outer-shell electron count from the last shell", () => {
    expect(BY_SYMBOL.get("na")!.valenceElectrons).toBe(1);
    expect(BY_SYMBOL.get("ne")!.valenceElectrons).toBe(8);
  });
});

describe("oxidation states", () => {
  it("curates sensible states for common elements", () => {
    expect(BY_SYMBOL.get("fe")!.oxidationStates).toEqual([2, 3, 6]);
    expect(BY_SYMBOL.get("fe")!.commonOxidationStates).toEqual([2, 3]);
    expect(BY_SYMBOL.get("na")!.commonOxidationStates).toEqual([1]);
    expect(BY_SYMBOL.get("f")!.oxidationStates).toEqual([-1]);
  });

  it("every common oxidation state is also in the full set", () => {
    for (const e of ELEMENTS) {
      for (const c of e.commonOxidationStates) {
        expect(e.oxidationStates, e.symbol).toContain(c);
      }
    }
  });
});

describe("radioactivity / synthetic flags", () => {
  it("flags Tc, Pm and Z>=84 as radioactive", () => {
    expect(BY_SYMBOL.get("tc")!.radioactive).toBe(true);
    expect(BY_SYMBOL.get("u")!.radioactive).toBe(true);
    expect(BY_SYMBOL.get("fe")!.radioactive).toBe(false);
  });
  it("flags transuranics as synthetic", () => {
    expect(BY_SYMBOL.get("am")!.synthetic).toBe(true);
    expect(BY_SYMBOL.get("u")!.synthetic).toBe(false);
  });
});

describe("search", () => {
  it("matches by symbol, name and number", () => {
    expect(searchElements("Fe").map((e) => e.symbol)).toContain("Fe");
    expect(searchElements("iron").map((e) => e.symbol)).toContain("Fe");
    expect(searchElements("26").map((e) => e.number)).toContain(26);
  });
  it("returns the full set for an empty query", () => {
    expect(searchElements("").length).toBe(118);
  });
});
