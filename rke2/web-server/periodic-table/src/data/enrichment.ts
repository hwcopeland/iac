// Graduate-level enrichment layered on top of the vendored dataset.
//
// The base dataset lacks oxidation states, so they are curated here from
// standard references (Greenwood & Earnshaw; CRC Handbook). `common` lists the
// oxidation state(s) most frequently encountered; `all` is the broader set of
// well-characterized states. Superheavy elements (Z > 108) have only predicted
// chemistry and are intentionally left sparse/empty.

import type { Category } from "../types";

export interface OxidationInfo {
  all: number[];
  common: number[];
}

// Keyed by element symbol.
export const OXIDATION_STATES: Record<string, OxidationInfo> = {
  H: { all: [-1, 1], common: [1] },
  He: { all: [], common: [] },
  Li: { all: [1], common: [1] },
  Be: { all: [2], common: [2] },
  B: { all: [3], common: [3] },
  C: { all: [-4, -3, -2, -1, 1, 2, 3, 4], common: [-4, 4] },
  N: { all: [-3, -2, -1, 1, 2, 3, 4, 5], common: [-3, 3, 5] },
  O: { all: [-2, -1, 1, 2], common: [-2] },
  F: { all: [-1], common: [-1] },
  Ne: { all: [], common: [] },
  Na: { all: [1], common: [1] },
  Mg: { all: [2], common: [2] },
  Al: { all: [3], common: [3] },
  Si: { all: [-4, 4], common: [4] },
  P: { all: [-3, 3, 5], common: [-3, 3, 5] },
  S: { all: [-2, 2, 4, 6], common: [-2, 4, 6] },
  Cl: { all: [-1, 1, 3, 5, 7], common: [-1, 1, 3, 5, 7] },
  Ar: { all: [], common: [] },
  K: { all: [1], common: [1] },
  Ca: { all: [2], common: [2] },
  Sc: { all: [3], common: [3] },
  Ti: { all: [2, 3, 4], common: [4] },
  V: { all: [2, 3, 4, 5], common: [5] },
  Cr: { all: [2, 3, 6], common: [3, 6] },
  Mn: { all: [2, 3, 4, 6, 7], common: [2, 4, 7] },
  Fe: { all: [2, 3, 6], common: [2, 3] },
  Co: { all: [2, 3], common: [2, 3] },
  Ni: { all: [2, 3], common: [2] },
  Cu: { all: [1, 2], common: [2] },
  Zn: { all: [2], common: [2] },
  Ga: { all: [3], common: [3] },
  Ge: { all: [-4, 2, 4], common: [4] },
  As: { all: [-3, 3, 5], common: [3, 5] },
  Se: { all: [-2, 2, 4, 6], common: [-2, 4, 6] },
  Br: { all: [-1, 1, 3, 5, 7], common: [-1, 1, 5] },
  Kr: { all: [2], common: [] },
  Rb: { all: [1], common: [1] },
  Sr: { all: [2], common: [2] },
  Y: { all: [3], common: [3] },
  Zr: { all: [4], common: [4] },
  Nb: { all: [3, 5], common: [5] },
  Mo: { all: [2, 3, 4, 5, 6], common: [4, 6] },
  Tc: { all: [4, 7], common: [7] },
  Ru: { all: [2, 3, 4, 6, 8], common: [3, 4] },
  Rh: { all: [3], common: [3] },
  Pd: { all: [2, 4], common: [2] },
  Ag: { all: [1], common: [1] },
  Cd: { all: [2], common: [2] },
  In: { all: [1, 3], common: [3] },
  Sn: { all: [-4, 2, 4], common: [2, 4] },
  Sb: { all: [-3, 3, 5], common: [3, 5] },
  Te: { all: [-2, 2, 4, 6], common: [4, 6] },
  I: { all: [-1, 1, 3, 5, 7], common: [-1, 1, 5, 7] },
  Xe: { all: [2, 4, 6, 8], common: [2, 4, 6] },
  Cs: { all: [1], common: [1] },
  Ba: { all: [2], common: [2] },
  La: { all: [3], common: [3] },
  Ce: { all: [3, 4], common: [3, 4] },
  Pr: { all: [3, 4], common: [3] },
  Nd: { all: [3], common: [3] },
  Pm: { all: [3], common: [3] },
  Sm: { all: [2, 3], common: [3] },
  Eu: { all: [2, 3], common: [3] },
  Gd: { all: [3], common: [3] },
  Tb: { all: [3, 4], common: [3] },
  Dy: { all: [3], common: [3] },
  Ho: { all: [3], common: [3] },
  Er: { all: [3], common: [3] },
  Tm: { all: [2, 3], common: [3] },
  Yb: { all: [2, 3], common: [3] },
  Lu: { all: [3], common: [3] },
  Hf: { all: [4], common: [4] },
  Ta: { all: [5], common: [5] },
  W: { all: [2, 3, 4, 5, 6], common: [4, 6] },
  Re: { all: [4, 6, 7], common: [7] },
  Os: { all: [3, 4, 6, 8], common: [4] },
  Ir: { all: [3, 4], common: [3, 4] },
  Pt: { all: [2, 4], common: [2, 4] },
  Au: { all: [1, 3], common: [3] },
  Hg: { all: [1, 2], common: [2] },
  Tl: { all: [1, 3], common: [1] },
  Pb: { all: [2, 4], common: [2] },
  Bi: { all: [3, 5], common: [3] },
  Po: { all: [-2, 2, 4, 6], common: [4] },
  At: { all: [-1, 1], common: [-1] },
  Rn: { all: [2], common: [] },
  Fr: { all: [1], common: [1] },
  Ra: { all: [2], common: [2] },
  Ac: { all: [3], common: [3] },
  Th: { all: [4], common: [4] },
  Pa: { all: [4, 5], common: [5] },
  U: { all: [3, 4, 5, 6], common: [6] },
  Np: { all: [3, 4, 5, 6, 7], common: [5] },
  Pu: { all: [3, 4, 5, 6], common: [4] },
  Am: { all: [2, 3, 4, 5, 6], common: [3] },
  Cm: { all: [3, 4], common: [3] },
  Bk: { all: [3, 4], common: [3] },
  Cf: { all: [3], common: [3] },
  Es: { all: [3], common: [3] },
  Fm: { all: [3], common: [3] },
  Md: { all: [2, 3], common: [3] },
  No: { all: [2, 3], common: [2] },
  Lr: { all: [3], common: [3] },
  Rf: { all: [4], common: [4] },
  Db: { all: [5], common: [5] },
  Sg: { all: [6], common: [6] },
  Bh: { all: [7], common: [7] },
  Hs: { all: [8], common: [8] },
  Mt: { all: [], common: [] },
  Ds: { all: [], common: [] },
  Rg: { all: [], common: [] },
  Cn: { all: [2], common: [] },
  Nh: { all: [], common: [] },
  Fl: { all: [], common: [] },
  Mc: { all: [], common: [] },
  Lv: { all: [], common: [] },
  Ts: { all: [], common: [] },
  Og: { all: [], common: [] },
};

/** Normalize the dataset's category string into our closed Category union. */
export function normalizeCategory(raw: string): Category {
  const c = raw.toLowerCase();
  if (c.includes("alkali metal")) return "alkali metal";
  if (c.includes("alkaline earth")) return "alkaline earth metal";
  if (c.includes("transition metal")) return "transition metal";
  if (c.includes("post-transition")) return "post-transition metal";
  if (c.includes("metalloid")) return "metalloid";
  if (c.includes("diatomic")) return "diatomic nonmetal";
  if (c.includes("polyatomic")) return "polyatomic nonmetal";
  if (c.includes("noble gas")) return "noble gas";
  if (c.includes("lanthanide")) return "lanthanide";
  if (c.includes("actinide")) return "actinide";
  return "unknown";
}

/** No stable isotope (standard convention: Tc, Pm, and everything from Po). */
export function isRadioactive(z: number): boolean {
  return z === 43 || z === 61 || z >= 84;
}

/** Not naturally occurring in usable quantities (lab-synthesized). */
export function isSynthetic(z: number): boolean {
  return z === 43 || z === 61 || z >= 93;
}
