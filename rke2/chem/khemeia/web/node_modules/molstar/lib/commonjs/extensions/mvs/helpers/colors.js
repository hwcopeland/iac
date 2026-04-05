"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MvsNamedColorLists = exports.MvsNamedColorDicts = void 0;
const element_symbol_1 = require("../../../mol-theme/color/element-symbol.js");
const residue_name_1 = require("../../../mol-theme/color/residue-name.js");
const secondary_structure_1 = require("../../../mol-theme/color/secondary-structure.js");
const color_1 = require("../../../mol-util/color/index.js");
const lists_1 = require("../../../mol-util/color/lists.js");
const object_1 = require("../../../mol-util/object.js");
const utils_1 = require("./utils.js");
/** Colors for amino acid groups, based on Clustal (https://www.jalview.org/help/html/colourSchemes/clustal.html) */
const AminoGroupColors = {
    aromatic: (0, utils_1.decodeColor)('#15A4A4'),
    hydrophobic: (0, utils_1.decodeColor)('#80A0F0'),
    polar: (0, utils_1.decodeColor)('#15C015'),
    positive: (0, utils_1.decodeColor)('#F01505'),
    negative: (0, utils_1.decodeColor)('#C048C0'),
    proline: (0, utils_1.decodeColor)('#C0C000'),
    cysteine: (0, utils_1.decodeColor)('#F08080'),
    glycine: (0, utils_1.decodeColor)('#F09048'),
};
/** Colors for individual amino acids, based on Clustal (https://www.jalview.org/help/html/colourSchemes/clustal.html), plus Jmol colors for nucleotides (http://jmol.sourceforge.net/jscolors/) */
const ResiduePropertyColors = {
    ...residue_name_1.ResidueNameColors,
    HIS: AminoGroupColors.aromatic,
    TYR: AminoGroupColors.aromatic,
    ALA: AminoGroupColors.hydrophobic,
    VAL: AminoGroupColors.hydrophobic,
    LEU: AminoGroupColors.hydrophobic,
    ILE: AminoGroupColors.hydrophobic,
    MET: AminoGroupColors.hydrophobic,
    PHE: AminoGroupColors.hydrophobic,
    TRP: AminoGroupColors.hydrophobic,
    SER: AminoGroupColors.polar,
    THR: AminoGroupColors.polar,
    ASN: AminoGroupColors.polar,
    GLN: AminoGroupColors.polar,
    LYS: AminoGroupColors.positive,
    ARG: AminoGroupColors.positive,
    ASP: AminoGroupColors.negative,
    GLU: AminoGroupColors.negative,
    PRO: AminoGroupColors.proline,
    CYS: AminoGroupColors.cysteine,
    GLY: AminoGroupColors.glycine,
};
/** Colors for secondary structure types, based on Jmol colors (http://jmol.sourceforge.net/jscolors/) */
const SecondaryStructureColors = {
    // Simple categories
    helix: secondary_structure_1.SecondaryStructureColors.alphaHelix,
    strand: secondary_structure_1.SecondaryStructureColors.betaStrand,
    turn: secondary_structure_1.SecondaryStructureColors.betaTurn,
    bend: secondary_structure_1.SecondaryStructureColors.bend,
    // DSSP categories
    H: secondary_structure_1.SecondaryStructureColors.alphaHelix,
    B: secondary_structure_1.SecondaryStructureColors.betaStrand,
    E: secondary_structure_1.SecondaryStructureColors.betaStrand,
    G: secondary_structure_1.SecondaryStructureColors.threeTenHelix,
    I: secondary_structure_1.SecondaryStructureColors.piHelix,
    P: (0, color_1.Color)(0xA00000), // Polyproline II helix, Jmol has no color for it
    T: secondary_structure_1.SecondaryStructureColors.betaTurn,
    S: secondary_structure_1.SecondaryStructureColors.bend,
};
exports.MvsNamedColorDicts = {
    ElementSymbol: (0, object_1.omitObjectKeys)(element_symbol_1.ElementSymbolColors, ['C']), // ommitting carbon color to allow easier combination of multiple color layers
    ResidueName: residue_name_1.ResidueNameColors,
    ResidueProperties: ResiduePropertyColors,
    SecondaryStructure: SecondaryStructureColors,
};
exports.MvsNamedColorLists = {
    // Sequential single-hue
    Reds: lists_1.ColorLists['reds'],
    Oranges: lists_1.ColorLists['oranges'],
    Greens: lists_1.ColorLists['greens'],
    Blues: lists_1.ColorLists['blues'],
    Purples: lists_1.ColorLists['purples'],
    Greys: lists_1.ColorLists['greys'],
    // Sequential multi-hue
    OrRd: lists_1.ColorLists['orange-red'],
    BuGn: lists_1.ColorLists['blue-green'],
    PuBuGn: lists_1.ColorLists['purple-blue-green'],
    GnBu: lists_1.ColorLists['green-blue'],
    PuBu: lists_1.ColorLists['purple-blue'],
    BuPu: lists_1.ColorLists['blue-purple'],
    RdPu: lists_1.ColorLists['red-purple'],
    PuRd: lists_1.ColorLists['purple-red'],
    YlOrRd: lists_1.ColorLists['yellow-orange-red'],
    YlOrBr: lists_1.ColorLists['yellow-orange-brown'],
    YlGn: lists_1.ColorLists['yellow-green'],
    YlGnBu: lists_1.ColorLists['yellow-green-blue'],
    Magma: lists_1.ColorLists['magma'],
    Inferno: lists_1.ColorLists['inferno'],
    Plasma: lists_1.ColorLists['plasma'],
    Viridis: lists_1.ColorLists['viridis'],
    Cividis: lists_1.ColorLists['cividis'],
    Turbo: lists_1.ColorLists['turbo'],
    Warm: lists_1.ColorLists['warm'],
    Cool: lists_1.ColorLists['cool'],
    CubehelixDefault: lists_1.ColorLists['cubehelix-default'],
    // Cyclical
    Rainbow: lists_1.ColorLists['rainbow'],
    Sinebow: lists_1.ColorLists['sinebow'],
    // Diverging
    RdBu: lists_1.ColorLists['red-blue'],
    RdGy: lists_1.ColorLists['red-grey'],
    PiYG: lists_1.ColorLists['pink-yellow-green'],
    BrBG: lists_1.ColorLists['brown-white-green'],
    PRGn: lists_1.ColorLists['purple-green'],
    PuOr: lists_1.ColorLists['purple-orange'],
    RdYlGn: lists_1.ColorLists['red-yellow-green'],
    RdYlBu: lists_1.ColorLists['red-yellow-blue'],
    Spectral: lists_1.ColorLists['spectral'],
    // Categorical
    Category10: lists_1.ColorLists['category-10'],
    Observable10: lists_1.ColorLists['observable-10'],
    Tableau10: lists_1.ColorLists['tableau-10'],
    Set1: lists_1.ColorLists['set-1'],
    Set2: lists_1.ColorLists['set-2'],
    Set3: lists_1.ColorLists['set-3'],
    Pastel1: lists_1.ColorLists['pastel-1'],
    Pastel2: lists_1.ColorLists['pastel-2'],
    Dark2: lists_1.ColorLists['dark-2'],
    Paired: lists_1.ColorLists['paired'],
    Accent: lists_1.ColorLists['accent'],
    // Additional lists, not standard for visualization in general, but commonly used for structures
    Chainbow: lists_1.ColorLists['turbo-no-black'],
};
