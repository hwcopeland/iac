// Domain types for the periodic table. The raw shape mirrors the vendored
// Bowserinator dataset (src/data/periodic-table.json, MIT); `Element` is the
// enriched, normalized record the UI consumes.

export interface RawElement {
  name: string;
  symbol: string;
  number: number;
  atomic_mass: number;
  category: string;
  period: number;
  group: number | null;
  phase: "Gas" | "Liquid" | "Solid" | "Unknown" | string;
  xpos: number;
  ypos: number;
  block: "s" | "p" | "d" | "f" | string;
  electron_configuration: string;
  electron_configuration_semantic: string;
  shells: number[];
  electron_affinity: number | null;
  electronegativity_pauling: number | null;
  ionization_energies: number[];
  density: number | null;
  melt: number | null; // K
  boil: number | null; // K
  molar_heat: number | null; // J/(mol·K)
  appearance: string | null;
  discovered_by: string | null;
  named_by: string | null;
  summary: string;
  "cpk-hex": string | null;
}

/** A periodic-table block. */
export type Block = "s" | "p" | "d" | "f";

/** Broad bonding/behaviour category used for coloring and grouping. */
export type Category =
  | "alkali metal"
  | "alkaline earth metal"
  | "transition metal"
  | "post-transition metal"
  | "metalloid"
  | "diatomic nonmetal"
  | "polyatomic nonmetal"
  | "noble gas"
  | "lanthanide"
  | "actinide"
  | "unknown";

/** Fully enriched, UI-facing element record. */
export interface Element {
  number: number;
  symbol: string;
  name: string;
  atomicMass: number;
  category: Category;
  categoryRaw: string;
  period: number;
  group: number | null;
  block: Block;
  phase: "Gas" | "Liquid" | "Solid" | "Unknown";

  /** Grid placement (1-indexed columns/rows), separated f-block layout. */
  xpos: number;
  ypos: number;

  electronConfiguration: string;
  electronConfigurationSemantic: string;
  shells: number[];
  valenceElectrons: number;

  electronegativity: number | null; // Pauling
  electronAffinity: number | null; // kJ/mol
  ionizationEnergies: number[]; // successive, kJ/mol

  density: number | null; // g/cm³ (solids/liquids), g/L for gases in source
  meltK: number | null;
  boilK: number | null;
  molarHeat: number | null; // J/(mol·K)

  /** Curated oxidation states; `common` is the subset most frequently seen. */
  oxidationStates: number[];
  commonOxidationStates: number[];

  radioactive: boolean;
  synthetic: boolean;

  appearance: string | null;
  discoveredBy: string | null;
  namedBy: string | null;
  summary: string;
  cpkHex: string | null;
}

/** A scalar property usable as a periodic-trend coloring dimension. */
export type TrendKey =
  | "category"
  | "phase"
  | "block"
  | "atomicMass"
  | "electronegativity"
  | "electronAffinity"
  | "anionicStability"
  | "firstIonizationEnergy"
  | "density"
  | "meltK"
  | "boilK";
