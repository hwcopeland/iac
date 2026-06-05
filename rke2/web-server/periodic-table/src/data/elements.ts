// Loads the vendored dataset and merges the graduate-level enrichment layer
// into the normalized `Element[]` the UI consumes. Element 119 (Uue) is
// hypothetical and excluded; the canonical table is Z = 1..118.

// elements.slim.json is generated from periodic-table.json by scripts/prune-data.mjs
// (npm run data:build, also wired as predev/prebuild). It carries only the fields
// the UI renders, keeping the raw dataset's unused image/spectra/source bytes out
// of the bundle.
import raw from "./elements.slim.json";
import type { Block, Element, RawElement } from "../types";
import {
  OXIDATION_STATES,
  isRadioactive,
  isSynthetic,
  normalizeCategory,
} from "./enrichment";

function normalizePhase(p: string): Element["phase"] {
  if (p === "Gas" || p === "Liquid" || p === "Solid") return p;
  return "Unknown";
}

function toElement(r: RawElement): Element {
  const ox = OXIDATION_STATES[r.symbol] ?? { all: [], common: [] };
  const shells = r.shells ?? [];
  return {
    number: r.number,
    symbol: r.symbol,
    name: r.name,
    atomicMass: r.atomic_mass,
    category: normalizeCategory(r.category),
    categoryRaw: r.category,
    period: r.period,
    group: r.group,
    block: (r.block as Block) ?? "s",
    phase: normalizePhase(r.phase),
    xpos: r.xpos,
    ypos: r.ypos,
    electronConfiguration: r.electron_configuration,
    electronConfigurationSemantic: r.electron_configuration_semantic,
    shells,
    valenceElectrons: shells.length ? shells[shells.length - 1] : 0,
    electronegativity: r.electronegativity_pauling,
    electronAffinity: r.electron_affinity,
    ionizationEnergies: r.ionization_energies ?? [],
    density: r.density,
    meltK: r.melt,
    boilK: r.boil,
    molarHeat: r.molar_heat,
    oxidationStates: ox.all,
    commonOxidationStates: ox.common,
    radioactive: isRadioactive(r.number),
    synthetic: isSynthetic(r.number),
    appearance: r.appearance,
    discoveredBy: r.discovered_by,
    namedBy: r.named_by,
    summary: r.summary,
    cpkHex: r["cpk-hex"],
  };
}

const dataset = raw as { elements: RawElement[] };

export const ELEMENTS: Element[] = dataset.elements
  .filter((e) => e.number >= 1 && e.number <= 118)
  .map(toElement)
  .sort((a, b) => a.number - b.number);

export const BY_NUMBER: Map<number, Element> = new Map(
  ELEMENTS.map((e) => [e.number, e]),
);

export const BY_SYMBOL: Map<string, Element> = new Map(
  ELEMENTS.map((e) => [e.symbol.toLowerCase(), e]),
);

/** Free-text search over symbol, name, atomic number. */
export function searchElements(query: string): Element[] {
  const q = query.trim().toLowerCase();
  if (!q) return ELEMENTS;
  const asNum = Number(q);
  return ELEMENTS.filter(
    (e) =>
      e.symbol.toLowerCase() === q ||
      e.name.toLowerCase().includes(q) ||
      e.symbol.toLowerCase().startsWith(q) ||
      (!Number.isNaN(asNum) && e.number === asNum),
  );
}
