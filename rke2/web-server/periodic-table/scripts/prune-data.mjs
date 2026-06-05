// Build-time data prune. The upstream Bowserinator dataset carries large fields
// the UI never reads (image/Bohr-model/spectra URLs, Wikipedia source, the wide
// f-block coordinates) — together ~50% of the dataset's bytes. Importing the raw
// JSON bundles all of it. This script projects each element down to the fields
// the app actually renders and writes a minified slim file that elements.ts
// imports. The raw periodic-table.json stays in the repo for provenance.
//
// Run via `npm run data:build` (also wired as predev/prebuild so it never drifts).

import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const dataDir = join(here, "..", "src", "data");

// Must stay in sync with RawElement in src/types.ts.
const KEEP = [
  "name",
  "symbol",
  "number",
  "atomic_mass",
  "category",
  "period",
  "group",
  "phase",
  "xpos",
  "ypos",
  "block",
  "electron_configuration",
  "electron_configuration_semantic",
  "shells",
  "electron_affinity",
  "electronegativity_pauling",
  "ionization_energies",
  "density",
  "melt",
  "boil",
  "molar_heat",
  "appearance",
  "discovered_by",
  "named_by",
  "summary",
  "cpk-hex",
];

const raw = JSON.parse(readFileSync(join(dataDir, "periodic-table.json"), "utf8"));

const elements = raw.elements
  .filter((e) => e.number >= 1 && e.number <= 118)
  .sort((a, b) => a.number - b.number)
  .map((e) => {
    const out = {};
    for (const k of KEEP) if (k in e) out[k] = e[k];
    return out;
  });

const slimPath = join(dataDir, "elements.slim.json");
writeFileSync(slimPath, JSON.stringify({ elements }));

const before = JSON.stringify(raw).length;
const after = readFileSync(slimPath, "utf8").length;
console.log(
  `prune-data: ${elements.length} elements · ${before} → ${after} bytes ` +
    `(-${(100 * (before - after) / before).toFixed(0)}%) → src/data/elements.slim.json`,
);
