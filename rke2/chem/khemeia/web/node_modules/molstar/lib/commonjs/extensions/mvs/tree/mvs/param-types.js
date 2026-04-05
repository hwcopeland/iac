"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Palette = exports.ContinuousPaletteDefaults = exports.ContinuousPalette = exports.DiscretePaletteDefaults = exports.DiscretePalette = exports.CategoricalPaletteDefaults = exports.CategoricalPalette = exports.ColorDictNameT = exports.ColorListNameT = exports.ColorT = exports.ColorNameT = exports.HexColorT = exports.StrList = exports.IntList = exports.FloatList = exports.PrimitivePositionT = exports.PrimitiveComponentExpressionT = exports.LabelAttachments = exports.Matrix = exports.Vector3 = exports.SchemaFormatT = exports.SchemaT = exports.ComponentExpressionT = exports.ComponentSelectorT = exports.StructureTypeT = exports.MolstarParseFormatT = exports.ParseFormatT = void 0;
exports.isVector3 = isVector3;
exports.isPrimitiveComponentExpressions = isPrimitiveComponentExpressions;
exports.isComponentExpression = isComponentExpression;
const tslib_1 = require("tslib");
const iots = tslib_1.__importStar(require("io-ts"));
const names_1 = require("../../../../mol-util/color/names.js");
const field_schema_1 = require("../generic/field-schema.js");
exports.ParseFormatT = (0, field_schema_1.literal)(
// trajectory
'mmcif', 'bcif', // +volumes
'pdb', 'pdbqt', 'gro', 'xyz', 'mol', 'sdf', 'mol2', 'lammpstrj', // + coordinates
// coordinates
'xtc', 'nctraj', 'dcd', 'trr', 
// topology
'psf', 'prmtop', 'top', 
// volumes
'map', 'dx', 'dxbin');
exports.MolstarParseFormatT = (0, field_schema_1.literal)(
// trajectory
'cif', // +volumes
'pdb', 'pdbqt', 'gro', 'xyz', 'mol', 'sdf', 'mol2', 'lammpstrj', 
// coordinates
'xtc', 'nctraj', 'dcd', 'trr', 
// topology
'psf', 'prmtop', 'top', 
// volumes
'map', 'dx', 'dxbin');
exports.StructureTypeT = (0, field_schema_1.literal)('model', 'assembly', 'symmetry', 'symmetry_mates');
exports.ComponentSelectorT = (0, field_schema_1.literal)('all', 'polymer', 'protein', 'nucleic', 'branched', 'ligand', 'ion', 'water', 'coarse');
/** `selector` parameter values for `component` node in MVS tree */
const _ComponentExpressionT = (0, field_schema_1.partial)({
    label_entity_id: field_schema_1.str,
    label_asym_id: field_schema_1.str,
    auth_asym_id: field_schema_1.str,
    label_seq_id: field_schema_1.int,
    auth_seq_id: field_schema_1.int,
    pdbx_PDB_ins_code: field_schema_1.str,
    beg_label_seq_id: field_schema_1.int,
    end_label_seq_id: field_schema_1.int,
    beg_auth_seq_id: field_schema_1.int,
    end_auth_seq_id: field_schema_1.int,
    label_comp_id: field_schema_1.str,
    auth_comp_id: field_schema_1.str,
    /** 0-based residue index in the source file */
    residue_index: field_schema_1.int, // TODO this is defined in Python builder but not supported by Molstar yet
    label_atom_id: field_schema_1.str,
    auth_atom_id: field_schema_1.str,
    type_symbol: field_schema_1.str,
    atom_id: field_schema_1.int,
    atom_index: field_schema_1.int,
    /** Instance identifier to distinguish instances of the same chain created by applying different symmetry operators,
     * like 'ASM-X0-1' for assemblies or '1_555' for crystals */
    instance_id: field_schema_1.str,
});
exports.ComponentExpressionT = _ComponentExpressionT;
exports.SchemaT = (0, field_schema_1.literal)('whole_structure', 'entity', 'chain', 'auth_chain', 'residue', 'auth_residue', 'residue_range', 'auth_residue_range', 'atom', 'auth_atom', 'all_atomic');
exports.SchemaFormatT = (0, field_schema_1.literal)('cif', 'bcif', 'json');
exports.Vector3 = (0, field_schema_1.tuple)([field_schema_1.float, field_schema_1.float, field_schema_1.float]);
/** Parameter values for matrix params, e.g. `rotation` */
exports.Matrix = (0, field_schema_1.list)(field_schema_1.float); // TODO impl custom types Matrix3x3 and Matrix4x4
exports.LabelAttachments = (0, field_schema_1.literal)('bottom-left', 'bottom-center', 'bottom-right', 'middle-left', 'middle-center', 'middle-right', 'top-left', 'top-center', 'top-right');
/** Primitives-related types */
const _PrimitiveComponentExpressionT = (0, field_schema_1.partial)({
    structure_ref: field_schema_1.str,
    expression_schema: exports.SchemaT,
    expressions: (0, field_schema_1.list)(exports.ComponentExpressionT),
});
exports.PrimitiveComponentExpressionT = _PrimitiveComponentExpressionT;
exports.PrimitivePositionT = (0, field_schema_1.union)(exports.Vector3, exports.ComponentExpressionT, exports.PrimitiveComponentExpressionT);
exports.FloatList = (0, field_schema_1.list)(field_schema_1.float);
exports.IntList = (0, field_schema_1.list)(field_schema_1.int);
exports.StrList = (0, field_schema_1.list)(field_schema_1.str);
exports.HexColorT = new iots.Type('HexColor', ((value) => typeof value === 'string'), (value, ctx) => isHexColorT(value) ? { _tag: 'Right', right: value } : { _tag: 'Left', left: [{ value: value, context: ctx, message: `"${value}" is not a valid hex color string` }] }, value => value);
/** Regular expression matching a hexadecimal color string, e.g. '#FF1100' or '#f10' */
const hexColorRegex = /^#([0-9A-F]{3}){1,2}$/i;
/** Decide if a string is a valid hexadecimal color string (6-digit or 3-digit, e.g. '#FF1100' or '#f10') */
function isHexColorT(str) {
    return typeof str === 'string' && hexColorRegex.test(str);
}
/** Named color string (e.g. 'red') for `color` parameter values for `color` node in MVS tree */
exports.ColorNameT = new iots.Type('ColorName', ((value) => typeof value === 'string'), (value, ctx) => isColorNameT(value) ? { _tag: 'Right', right: value } : { _tag: 'Left', left: [{ value: value, context: ctx, message: `"${value}" is not a valid color name` }] }, value => value);
/** Decide if a string is a valid named color string */
function isColorNameT(str) {
    return str in names_1.ColorNames;
}
exports.ColorT = (0, field_schema_1.union)(exports.ColorNameT, exports.HexColorT);
// Type helpers
function isVector3(x) {
    return !!x && Array.isArray(x) && x.length === 3 && typeof x[0] === 'number';
}
function isPrimitiveComponentExpressions(x) {
    return !!x && Array.isArray(x.expressions);
}
function isComponentExpression(x) {
    return !!x && typeof x === 'object' && !x.expressions;
}
exports.ColorListNameT = (0, field_schema_1.literal)(
// Color lists from https://observablehq.com/@d3/color-schemes (definitions: https://colorbrewer2.org/export/colorbrewer.js)
// Sequential single-hue
'Reds', 'Oranges', 'Greens', 'Blues', 'Purples', 'Greys', 
// Sequential multi-hue
'OrRd', 'BuGn', 'PuBuGn', 'GnBu', 'PuBu', 'BuPu', 'RdPu', 'PuRd', 'YlOrRd', 'YlOrBr', 'YlGn', 'YlGnBu', 'Magma', 'Inferno', 'Plasma', 'Viridis', 'Cividis', 'Turbo', 'Warm', 'Cool', 'CubehelixDefault', 
// Cyclical
'Rainbow', 'Sinebow', 
// Diverging
'RdBu', 'RdGy', 'PiYG', 'BrBG', 'PRGn', 'PuOr', 'RdYlGn', 'RdYlBu', 'Spectral', 
// Categorical
'Category10', 'Observable10', 'Tableau10', 'Set1', 'Set2', 'Set3', 'Pastel1', 'Pastel2', 'Dark2', 'Paired', 'Accent', 
// Additional lists, not standard for visualization in general, but commonly used for structures
'Chainbow');
exports.ColorDictNameT = (0, field_schema_1.literal)('ElementSymbol', 'ResidueName', 'ResidueProperties', 'SecondaryStructure');
const _CategoricalPalette = (0, field_schema_1.object)({
    kind: (0, field_schema_1.literal)('categorical'),
}, 
// Optionals:
{
    colors: (0, field_schema_1.union)(exports.ColorListNameT, exports.ColorDictNameT, (0, field_schema_1.list)(exports.ColorT), (0, field_schema_1.dict)(field_schema_1.str, exports.ColorT)),
    /** Repeat color list once all colors are depleted (only applies if `colors` is a list or a color list name). */
    repeat_color_list: field_schema_1.bool,
    /** Sort actual annotation values before assigning colors from a list (none = take values in order of their first occurrence). */
    sort: (0, field_schema_1.literal)('none', 'lexical', 'numeric'),
    /** Sort direction. */
    sort_direction: (0, field_schema_1.literal)('ascending', 'descending'),
    /** Treat annotation values as case-insensitive strings. */
    case_insensitive: field_schema_1.bool,
    /** Color to use when a) `colors` is a dictionary (or a color dictionary name) and given key is not present, or b) `colors` is a list (or a color list name) and there are more actual annotation values than listed colors and `repeat_color_list` is not true. */
    missing_color: (0, field_schema_1.nullable)(exports.ColorT),
});
exports.CategoricalPalette = _CategoricalPalette;
exports.CategoricalPaletteDefaults = {
    kind: 'categorical',
    colors: 'Category10', // this is also default for categorical in Matplotlib
    repeat_color_list: false,
    sort: 'none',
    sort_direction: 'ascending',
    case_insensitive: false,
    missing_color: null,
};
const _DiscretePalette = (0, field_schema_1.object)({
    kind: (0, field_schema_1.literal)('discrete'),
}, 
// Optionals:
{
    /** Define colors for the discrete color palette and optionally corresponding checkpoints.
     * Checkpoints refer to the values normalized to interval [0, 1] if `mode` is `"normalized"` (default), or to the values directly if `mode` is `"absolute"`.
     * If checkpoints are not provided, they will created automatically (uniformly distributed over interval [0, 1]).
     * If 1 checkpoint is provided for each color, then the color applies to values from this checkpoint (inclusive) until the next listed checkpoint (exclusive); the last color applies until Infinity.
     * If 2 checkpoints are provided for each color, then the color applies to values from the first until the second checkpoint (inclusive); null means +/-Infinity; if ranges overlap, the later listed takes precedence.
     */
    colors: (0, field_schema_1.union)(exports.ColorListNameT, (0, field_schema_1.list)(exports.ColorT), (0, field_schema_1.list)((0, field_schema_1.tuple)([exports.ColorT, field_schema_1.float])), (0, field_schema_1.list)((0, field_schema_1.tuple)([(0, field_schema_1.nullable)(exports.ColorT), (0, field_schema_1.nullable)(field_schema_1.float), (0, field_schema_1.nullable)(field_schema_1.float)]))),
    /** Reverse order of `colors` list. Only has effect when `colors` is a color list name or a color list without explicit checkpoints. */
    reverse: field_schema_1.bool,
    /** Defines whether the annotation values should be normalized before assigning color based on checkpoints in `colors` (`x_normalized = (x - x_min) / (x_max - x_min)`, where `[x_min, x_max]` are either `value_domain` if provided, or the lowest and the highest value encountered in the annotation). Default is `"normalized"`. */
    mode: (0, field_schema_1.literal)('normalized', 'absolute'),
    /** Defines `x_min` and `x_max` for normalization of annotation values. Either can be `null`, meaning that minimum/maximum of the actual values will be used. Only used when `mode` is `"normalized"`. */
    value_domain: (0, field_schema_1.tuple)([(0, field_schema_1.nullable)(field_schema_1.float), (0, field_schema_1.nullable)(field_schema_1.float)]),
});
exports.DiscretePalette = _DiscretePalette;
exports.DiscretePaletteDefaults = {
    kind: 'discrete',
    colors: 'YlGn', // YlGn was selected as default because (a) Matplotlib's default Viridis looks ugly in 3D and (b) YlGn does not contain white, so it's easier to see that it's doing something even when values are in wrong range
    reverse: false,
    mode: 'normalized',
    value_domain: [null, null],
};
const _ContinuousPalette = (0, field_schema_1.object)({
    kind: (0, field_schema_1.literal)('continuous'),
}, 
// Optionals:
{
    /** Define colors for the continuous color palette and optionally corresponding checkpoints (i.e. annotation values that are mapped to each color).
     * Checkpoints refer to the values normalized to interval [0, 1] if `mode` is `"normalized"` (default), or to the values directly if `mode` is `"absolute"`.
     * If checkpoints are not provided, they will created automatically (uniformly distributed over interval [0, 1]). */
    colors: (0, field_schema_1.union)(exports.ColorListNameT, (0, field_schema_1.list)(exports.ColorT), (0, field_schema_1.list)((0, field_schema_1.tuple)([exports.ColorT, field_schema_1.float]))),
    /** Reverse order of `colors` list. Only has effect when `colors` is a color list name or a color list without explicit checkpoints. */
    reverse: field_schema_1.bool,
    /** Defines whether the annotation values should be normalized before assigning color based on checkpoints in `colors` (`x_normalized = (x - x_min) / (x_max - x_min)`, where `[x_min, x_max]` are either `value_domain` if provided, or the lowest and the highest value encountered in the annotation). Default is `"normalized"`. */
    mode: (0, field_schema_1.literal)('normalized', 'absolute'),
    /** Defines `x_min` and `x_max` for normalization of annotation values. Either can be `null`, meaning that minimum/maximum of the actual values will be used. Only used when `mode` is `"normalized"`. */
    value_domain: (0, field_schema_1.tuple)([(0, field_schema_1.nullable)(field_schema_1.float), (0, field_schema_1.nullable)(field_schema_1.float)]),
    /** Color to use for values below the lowest checkpoint. 'auto' means color of the lowest checkpoint. */
    underflow_color: (0, field_schema_1.nullable)((0, field_schema_1.union)((0, field_schema_1.literal)('auto'), exports.ColorT)),
    /** Color to use for values above the highest checkpoint. 'auto' means color of the highest checkpoint. */
    overflow_color: (0, field_schema_1.nullable)((0, field_schema_1.union)((0, field_schema_1.literal)('auto'), exports.ColorT)),
});
exports.ContinuousPalette = _ContinuousPalette;
exports.ContinuousPaletteDefaults = {
    kind: 'continuous',
    colors: 'YlGn', // YlGn was selected as default because (a) Matplotlib's default Viridis looks ugly in 3D and (b) YlGn does not contain white, so it's easier to see that it's doing something even when values are in wrong range
    reverse: false,
    mode: 'normalized',
    value_domain: [null, null],
    underflow_color: null,
    overflow_color: null,
};
// TODO consider spreading the palette param directly into color_from_uri/color_from_source params (though this will be tricky)
// TODO consider implementing some kind of recursion for object-typed params to achieve smart error messages and default value handling
exports.Palette = (0, field_schema_1.union)(exports.CategoricalPalette, exports.DiscretePalette, exports.ContinuousPalette);
