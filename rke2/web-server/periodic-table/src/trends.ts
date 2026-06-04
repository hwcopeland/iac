// Periodic-trend coloring engine. Pure functions only (no DOM) so the scales
// can be unit-tested. Two families of trends:
//   - categorical (category, phase, block) → fixed palette
//   - numeric (atomic mass, electronegativity, ionization energy, …) → a
//     sequential scale spanning the observed min/max, with missing data greyed.

import type { Element, TrendKey } from "./types";

export const CATEGORY_COLORS: Record<string, string> = {
  "alkali metal": "#ff6b6b",
  "alkaline earth metal": "#ffa94d",
  "transition metal": "#ffd43b",
  "post-transition metal": "#69db7c",
  metalloid: "#38d9a9",
  "diatomic nonmetal": "#4dabf7",
  "polyatomic nonmetal": "#3bc9db",
  "noble gas": "#b197fc",
  lanthanide: "#f783ac",
  actinide: "#e599f7",
  unknown: "#868e96",
};

export const PHASE_COLORS: Record<string, string> = {
  Gas: "#74c0fc",
  Liquid: "#63e6be",
  Solid: "#ffa94d",
  Unknown: "#868e96",
};

export const BLOCK_COLORS: Record<string, string> = {
  s: "#ff8787",
  p: "#4dabf7",
  d: "#ffd43b",
  f: "#f783ac",
};

const MISSING_COLOR = "#2b2f36";

export interface TrendMeta {
  key: TrendKey;
  label: string;
  kind: "categorical" | "numeric";
  unit?: string;
}

export const TRENDS: TrendMeta[] = [
  { key: "category", label: "Category", kind: "categorical" },
  { key: "phase", label: "Phase (STP)", kind: "categorical" },
  { key: "block", label: "Block", kind: "categorical" },
  { key: "atomicMass", label: "Atomic mass", kind: "numeric", unit: "u" },
  { key: "electronegativity", label: "Electronegativity", kind: "numeric", unit: "Pauling" },
  { key: "electronAffinity", label: "Electron affinity", kind: "numeric", unit: "kJ/mol" },
  { key: "firstIonizationEnergy", label: "1st ionization energy", kind: "numeric", unit: "kJ/mol" },
  { key: "density", label: "Density", kind: "numeric", unit: "g/cm³" },
  { key: "meltK", label: "Melting point", kind: "numeric", unit: "K" },
  { key: "boilK", label: "Boiling point", kind: "numeric", unit: "K" },
];

export function trendMeta(key: TrendKey): TrendMeta {
  const m = TRENDS.find((t) => t.key === key);
  if (!m) throw new Error(`unknown trend: ${key}`);
  return m;
}

/** Extract the numeric value for a numeric trend, or null when unavailable. */
export function numericValue(e: Element, key: TrendKey): number | null {
  switch (key) {
    case "atomicMass":
      return e.atomicMass ?? null;
    case "electronegativity":
      return e.electronegativity;
    case "electronAffinity":
      return e.electronAffinity;
    case "firstIonizationEnergy":
      return e.ionizationEnergies.length ? e.ionizationEnergies[0] : null;
    case "density":
      return e.density;
    case "meltK":
      return e.meltK;
    case "boilK":
      return e.boilK;
    default:
      return null;
  }
}

export interface Range {
  min: number;
  max: number;
}

/** Observed [min, max] for a numeric trend across the given elements. */
export function numericRange(elements: Element[], key: TrendKey): Range {
  let min = Infinity;
  let max = -Infinity;
  for (const e of elements) {
    const v = numericValue(e, key);
    if (v == null || Number.isNaN(v)) continue;
    if (v < min) min = v;
    if (v > max) max = v;
  }
  if (min === Infinity) return { min: 0, max: 1 };
  if (min === max) return { min, max: min + 1 };
  return { min, max };
}

// Sequential scale stops (low → high), perceptually increasing.
const SCALE_STOPS = [
  [13, 27, 42], // deep navy
  [22, 96, 138],
  [38, 166, 154],
  [241, 196, 15],
  [231, 76, 60], // warm red
];

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

/** Map t∈[0,1] to a hex color along the sequential scale. */
export function scaleColor(t: number): string {
  const clamped = Math.max(0, Math.min(1, t));
  const seg = clamped * (SCALE_STOPS.length - 1);
  const i = Math.min(Math.floor(seg), SCALE_STOPS.length - 2);
  const f = seg - i;
  const [r1, g1, b1] = SCALE_STOPS[i];
  const [r2, g2, b2] = SCALE_STOPS[i + 1];
  const r = Math.round(lerp(r1, r2, f));
  const g = Math.round(lerp(g1, g2, f));
  const b = Math.round(lerp(b1, b2, f));
  return `rgb(${r}, ${g}, ${b})`;
}

/** Color for an element under the active trend. */
export function colorFor(e: Element, key: TrendKey, range: Range): string {
  const meta = trendMeta(key);
  if (meta.kind === "categorical") {
    if (key === "category") return CATEGORY_COLORS[e.category] ?? MISSING_COLOR;
    if (key === "phase") return PHASE_COLORS[e.phase] ?? MISSING_COLOR;
    if (key === "block") return BLOCK_COLORS[e.block] ?? MISSING_COLOR;
  }
  const v = numericValue(e, key);
  if (v == null || Number.isNaN(v)) return MISSING_COLOR;
  const t = (v - range.min) / (range.max - range.min);
  return scaleColor(t);
}

export { MISSING_COLOR };
