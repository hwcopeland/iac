import { describe, expect, it } from "vitest";
import { fmtNum, kToC, kToF, parseConfig, signed } from "./format";

describe("fmtNum", () => {
  it("renders an em dash for null/NaN", () => {
    expect(fmtNum(null)).toBe("—");
    expect(fmtNum(NaN)).toBe("—");
  });
  it("rounds to the requested precision", () => {
    expect(fmtNum(1.23456, 2)).toBe("1.23");
    expect(fmtNum(55.8452, 4)).toBe("55.8452");
  });
  it("uses exponential notation for very large/small values", () => {
    expect(fmtNum(123456)).toMatch(/e\+/);
    expect(fmtNum(0.0001)).toMatch(/e-/);
  });
});

describe("temperature conversions", () => {
  it("converts kelvin to celsius and fahrenheit", () => {
    expect(kToC(273.15)).toBeCloseTo(0);
    expect(kToF(273.15)).toBeCloseTo(32);
    expect(kToC(null)).toBeNull();
  });
});

describe("signed", () => {
  it("prefixes a sign", () => {
    expect(signed(2)).toBe("+2");
    expect(signed(-1)).toBe("-1");
    expect(signed(0)).toBe("0");
  });
});

describe("parseConfig", () => {
  it("splits a full configuration into shell/count tokens", () => {
    const t = parseConfig("1s2 2s2 2p6");
    expect(t).toEqual([
      { shell: "1s", count: 2 },
      { shell: "2s", count: 2 },
      { shell: "2p", count: 6 },
    ]);
  });
  it("captures a noble-gas core token", () => {
    const t = parseConfig("[Ne] 3s1");
    expect(t[0]).toEqual({ core: "[Ne]" });
    expect(t[1]).toEqual({ shell: "3s", count: 1 });
  });
});
