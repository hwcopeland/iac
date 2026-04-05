/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import * as iots from 'io-ts';
import { ValueFor } from '../generic/field-schema.js';
/** `format` parameter values for `parse` node in MVS tree */
export type ParseFormatT = 'mmcif' | 'bcif' | 'pdb' | 'pdbqt' | 'gro' | 'xyz' | 'mol' | 'sdf' | 'mol2' | 'lammpstrj' | 'xtc' | 'nctraj' | 'dcd' | 'trr' | 'psf' | 'prmtop' | 'top' | 'map' | 'dx' | 'dxbin';
export declare const ParseFormatT: iots.Type<ParseFormatT, ParseFormatT, unknown>;
/** `format` parameter values for `parse` node in Molstar tree */
export type MolstarParseFormatT = 'cif' | 'pdb' | 'pdbqt' | 'gro' | 'xyz' | 'mol' | 'sdf' | 'mol2' | 'lammpstrj' | 'xtc' | 'nctraj' | 'dcd' | 'trr' | 'psf' | 'prmtop' | 'top' | 'map' | 'dx' | 'dxbin';
export declare const MolstarParseFormatT: iots.Type<MolstarParseFormatT, MolstarParseFormatT, unknown>;
/** `kind` parameter values for `structure` node in MVS tree */
export type StructureTypeT = 'model' | 'assembly' | 'symmetry' | 'symmetry_mates';
export declare const StructureTypeT: iots.Type<StructureTypeT, StructureTypeT, unknown>;
/** `selector` parameter values for `component` node in MVS tree */
export type ComponentSelectorT = 'all' | 'polymer' | 'protein' | 'nucleic' | 'branched' | 'ligand' | 'ion' | 'water' | 'coarse';
export declare const ComponentSelectorT: iots.Type<ComponentSelectorT, ComponentSelectorT, unknown>;
/** `selector` parameter values for `component` node in MVS tree */
declare const _ComponentExpressionT: iots.PartialC<{
    label_entity_id: iots.StringC;
    label_asym_id: iots.StringC;
    auth_asym_id: iots.StringC;
    label_seq_id: iots.RefinementC<iots.NumberC, number>;
    auth_seq_id: iots.RefinementC<iots.NumberC, number>;
    pdbx_PDB_ins_code: iots.StringC;
    beg_label_seq_id: iots.RefinementC<iots.NumberC, number>;
    end_label_seq_id: iots.RefinementC<iots.NumberC, number>;
    beg_auth_seq_id: iots.RefinementC<iots.NumberC, number>;
    end_auth_seq_id: iots.RefinementC<iots.NumberC, number>;
    label_comp_id: iots.StringC;
    auth_comp_id: iots.StringC;
    /** 0-based residue index in the source file */
    residue_index: iots.RefinementC<iots.NumberC, number>;
    label_atom_id: iots.StringC;
    auth_atom_id: iots.StringC;
    type_symbol: iots.StringC;
    atom_id: iots.RefinementC<iots.NumberC, number>;
    atom_index: iots.RefinementC<iots.NumberC, number>;
    /** Instance identifier to distinguish instances of the same chain created by applying different symmetry operators,
     * like 'ASM-X0-1' for assemblies or '1_555' for crystals */
    instance_id: iots.StringC;
}>;
/** `selector` parameter values for `component` node in MVS tree */
export interface ComponentExpressionT extends ValueFor<typeof _ComponentExpressionT> {
}
export declare const ComponentExpressionT: iots.Type<ComponentExpressionT>;
/** `schema` parameter values for `*_from_uri` and `*_from_source` nodes in MVS tree */
export type SchemaT = 'whole_structure' | 'entity' | 'chain' | 'auth_chain' | 'residue' | 'auth_residue' | 'residue_range' | 'auth_residue_range' | 'atom' | 'auth_atom' | 'all_atomic';
export declare const SchemaT: iots.Type<SchemaT, SchemaT, unknown>;
/** `format` parameter values for `*_from_uri` nodes in MVS tree */
export type SchemaFormatT = 'cif' | 'bcif' | 'json';
export declare const SchemaFormatT: iots.Type<SchemaFormatT, SchemaFormatT, unknown>;
/** Parameter values for vector params, e.g. `position` */
export type Vector3 = [number, number, number];
export declare const Vector3: iots.Type<Vector3>;
/** Parameter values for matrix params, e.g. `rotation` */
export declare const Matrix: iots.ArrayC<iots.NumberC>;
export type LabelAttachments = 'bottom-left' | 'bottom-center' | 'bottom-right' | 'middle-left' | 'middle-center' | 'middle-right' | 'top-left' | 'top-center' | 'top-right';
export declare const LabelAttachments: iots.Type<LabelAttachments, LabelAttachments, unknown>;
/** Primitives-related types */
declare const _PrimitiveComponentExpressionT: iots.PartialC<{
    structure_ref: iots.StringC;
    expression_schema: iots.Type<SchemaT, SchemaT, unknown>;
    expressions: iots.ArrayC<iots.Type<ComponentExpressionT, ComponentExpressionT, unknown>>;
}>;
/** Primitives-related types */
export interface PrimitiveComponentExpressionT extends ValueFor<typeof _PrimitiveComponentExpressionT> {
}
export declare const PrimitiveComponentExpressionT: iots.Type<PrimitiveComponentExpressionT>;
export declare const PrimitivePositionT: iots.UnionC<[iots.Type<Vector3, Vector3, unknown>, iots.Type<ComponentExpressionT, ComponentExpressionT, unknown>, iots.Type<PrimitiveComponentExpressionT, PrimitiveComponentExpressionT, unknown>]>;
export type PrimitivePositionT = ValueFor<typeof PrimitivePositionT>;
export declare const FloatList: iots.ArrayC<iots.NumberC>;
export declare const IntList: iots.ArrayC<iots.RefinementC<iots.NumberC, number>>;
export declare const StrList: iots.ArrayC<iots.StringC>;
/** Hexadecimal color string, e.g. '#FF1100' (the type matches more than just valid HexColor strings) */
export type HexColorT = `#${string}`;
export declare const HexColorT: iots.Type<`#${string}`, `#${string}`, unknown>;
/** Named color string (e.g. 'red') for `color` parameter values for `color` node in MVS tree */
export declare const ColorNameT: iots.Type<ColorNameT, ColorNameT, unknown>;
export type ColorNameT = 'aliceblue' | 'antiquewhite' | 'aqua' | 'aquamarine' | 'azure' | 'beige' | 'bisque' | 'black' | 'blanchedalmond' | 'blue' | 'blueviolet' | 'brown' | 'burlywood' | 'cadetblue' | 'chartreuse' | 'chocolate' | 'coral' | 'cornflower' | 'cornflowerblue' | 'cornsilk' | 'crimson' | 'cyan' | 'darkblue' | 'darkcyan' | 'darkgoldenrod' | 'darkgray' | 'darkgreen' | 'darkgrey' | 'darkkhaki' | 'darkmagenta' | 'darkolivegreen' | 'darkorange' | 'darkorchid' | 'darkred' | 'darksalmon' | 'darkseagreen' | 'darkslateblue' | 'darkslategray' | 'darkslategrey' | 'darkturquoise' | 'darkviolet' | 'deeppink' | 'deepskyblue' | 'dimgray' | 'dimgrey' | 'dodgerblue' | 'firebrick' | 'floralwhite' | 'forestgreen' | 'fuchsia' | 'gainsboro' | 'ghostwhite' | 'gold' | 'goldenrod' | 'gray' | 'green' | 'greenyellow' | 'grey' | 'honeydew' | 'hotpink' | 'indianred' | 'indigo' | 'ivory' | 'khaki' | 'laserlemon' | 'lavender' | 'lavenderblush' | 'lawngreen' | 'lemonchiffon' | 'lightblue' | 'lightcoral' | 'lightcyan' | 'lightgoldenrod' | 'lightgoldenrodyellow' | 'lightgray' | 'lightgreen' | 'lightgrey' | 'lightpink' | 'lightsalmon' | 'lightseagreen' | 'lightskyblue' | 'lightslategray' | 'lightslategrey' | 'lightsteelblue' | 'lightyellow' | 'lime' | 'limegreen' | 'linen' | 'magenta' | 'maroon' | 'maroon2' | 'maroon3' | 'mediumaquamarine' | 'mediumblue' | 'mediumorchid' | 'mediumpurple' | 'mediumseagreen' | 'mediumslateblue' | 'mediumspringgreen' | 'mediumturquoise' | 'mediumvioletred' | 'midnightblue' | 'mintcream' | 'mistyrose' | 'moccasin' | 'navajowhite' | 'navy' | 'oldlace' | 'olive' | 'olivedrab' | 'orange' | 'orangered' | 'orchid' | 'palegoldenrod' | 'palegreen' | 'paleturquoise' | 'palevioletred' | 'papayawhip' | 'peachpuff' | 'peru' | 'pink' | 'plum' | 'powderblue' | 'purple' | 'purple2' | 'purple3' | 'rebeccapurple' | 'red' | 'rosybrown' | 'royalblue' | 'saddlebrown' | 'salmon' | 'sandybrown' | 'seagreen' | 'seashell' | 'sienna' | 'silver' | 'skyblue' | 'slateblue' | 'slategray' | 'slategrey' | 'snow' | 'springgreen' | 'steelblue' | 'tan' | 'teal' | 'thistle' | 'tomato' | 'turquoise' | 'violet' | 'wheat' | 'white' | 'whitesmoke' | 'yellow' | 'yellowgreen';
/** `color` parameter values for `color` node in MVS tree */
export type ColorT = ColorNameT | HexColorT;
export declare const ColorT: iots.Type<ColorT>;
export declare function isVector3(x: any): x is Vector3;
export declare function isPrimitiveComponentExpressions(x: any): x is PrimitiveComponentExpressionT;
export declare function isComponentExpression(x: any): x is ComponentExpressionT;
export type ColorListNameT = 'Reds' | 'Oranges' | 'Greens' | 'Blues' | 'Purples' | 'Greys' | 'OrRd' | 'BuGn' | 'PuBuGn' | 'GnBu' | 'PuBu' | 'BuPu' | 'RdPu' | 'PuRd' | 'YlOrRd' | 'YlOrBr' | 'YlGn' | 'YlGnBu' | 'Magma' | 'Inferno' | 'Plasma' | 'Viridis' | 'Cividis' | 'Turbo' | 'Warm' | 'Cool' | 'CubehelixDefault' | 'Rainbow' | 'Sinebow' | 'RdBu' | 'RdGy' | 'PiYG' | 'BrBG' | 'PRGn' | 'PuOr' | 'RdYlGn' | 'RdYlBu' | 'Spectral' | 'Category10' | 'Observable10' | 'Tableau10' | 'Set1' | 'Set2' | 'Set3' | 'Pastel1' | 'Pastel2' | 'Dark2' | 'Paired' | 'Accent' | 'Chainbow';
export declare const ColorListNameT: iots.Type<ColorListNameT, ColorListNameT, unknown>;
export type ColorDictNameT = 'ElementSymbol' | 'ResidueName' | 'ResidueProperties' | 'SecondaryStructure';
export declare const ColorDictNameT: iots.Type<ColorDictNameT, ColorDictNameT, unknown>;
declare const _CategoricalPalette: iots.IntersectionC<[iots.TypeC<{
    kind: iots.Type<"categorical", "categorical", unknown>;
}>, iots.PartialC<{
    colors: iots.UnionC<[iots.Type<ColorListNameT, ColorListNameT, unknown>, iots.Type<ColorDictNameT, ColorDictNameT, unknown>, iots.ArrayC<iots.Type<ColorT, ColorT, unknown>>, iots.RecordC<iots.StringC, iots.Type<ColorT, ColorT, unknown>>]>;
    /** Repeat color list once all colors are depleted (only applies if `colors` is a list or a color list name). */
    repeat_color_list: iots.BooleanC;
    /** Sort actual annotation values before assigning colors from a list (none = take values in order of their first occurrence). */
    sort: iots.Type<"none" | "numeric" | "lexical", "none" | "numeric" | "lexical", unknown>;
    /** Sort direction. */
    sort_direction: iots.Type<"ascending" | "descending", "ascending" | "descending", unknown>;
    /** Treat annotation values as case-insensitive strings. */
    case_insensitive: iots.BooleanC;
    /** Color to use when a) `colors` is a dictionary (or a color dictionary name) and given key is not present, or b) `colors` is a list (or a color list name) and there are more actual annotation values than listed colors and `repeat_color_list` is not true. */
    missing_color: iots.Type<ColorT | null, ColorT | null, unknown>;
}>]>;
export interface CategoricalPalette extends ValueFor<typeof _CategoricalPalette> {
}
export declare const CategoricalPalette: iots.Type<CategoricalPalette>;
export declare const CategoricalPaletteDefaults: Required<CategoricalPalette>;
declare const _DiscretePalette: iots.IntersectionC<[iots.TypeC<{
    kind: iots.Type<"discrete", "discrete", unknown>;
}>, iots.PartialC<{
    /** Define colors for the discrete color palette and optionally corresponding checkpoints.
     * Checkpoints refer to the values normalized to interval [0, 1] if `mode` is `"normalized"` (default), or to the values directly if `mode` is `"absolute"`.
     * If checkpoints are not provided, they will created automatically (uniformly distributed over interval [0, 1]).
     * If 1 checkpoint is provided for each color, then the color applies to values from this checkpoint (inclusive) until the next listed checkpoint (exclusive); the last color applies until Infinity.
     * If 2 checkpoints are provided for each color, then the color applies to values from the first until the second checkpoint (inclusive); null means +/-Infinity; if ranges overlap, the later listed takes precedence.
     */
    colors: iots.UnionC<[iots.Type<ColorListNameT, ColorListNameT, unknown>, iots.ArrayC<iots.Type<ColorT, ColorT, unknown>>, iots.ArrayC<iots.TupleC<[iots.Type<ColorT, ColorT, unknown>, iots.NumberC]>>, iots.ArrayC<iots.TupleC<[iots.Type<ColorT | null, ColorT | null, unknown>, iots.Type<number | null, number | null, unknown>, iots.Type<number | null, number | null, unknown>]>>]>;
    /** Reverse order of `colors` list. Only has effect when `colors` is a color list name or a color list without explicit checkpoints. */
    reverse: iots.BooleanC;
    /** Defines whether the annotation values should be normalized before assigning color based on checkpoints in `colors` (`x_normalized = (x - x_min) / (x_max - x_min)`, where `[x_min, x_max]` are either `value_domain` if provided, or the lowest and the highest value encountered in the annotation). Default is `"normalized"`. */
    mode: iots.Type<"absolute" | "normalized", "absolute" | "normalized", unknown>;
    /** Defines `x_min` and `x_max` for normalization of annotation values. Either can be `null`, meaning that minimum/maximum of the actual values will be used. Only used when `mode` is `"normalized"`. */
    value_domain: iots.TupleC<[iots.Type<number | null, number | null, unknown>, iots.Type<number | null, number | null, unknown>]>;
}>]>;
export interface DiscretePalette extends ValueFor<typeof _DiscretePalette> {
}
export declare const DiscretePalette: iots.Type<DiscretePalette>;
export declare const DiscretePaletteDefaults: Required<DiscretePalette>;
declare const _ContinuousPalette: iots.IntersectionC<[iots.TypeC<{
    kind: iots.Type<"continuous", "continuous", unknown>;
}>, iots.PartialC<{
    /** Define colors for the continuous color palette and optionally corresponding checkpoints (i.e. annotation values that are mapped to each color).
     * Checkpoints refer to the values normalized to interval [0, 1] if `mode` is `"normalized"` (default), or to the values directly if `mode` is `"absolute"`.
     * If checkpoints are not provided, they will created automatically (uniformly distributed over interval [0, 1]). */
    colors: iots.UnionC<[iots.Type<ColorListNameT, ColorListNameT, unknown>, iots.ArrayC<iots.Type<ColorT, ColorT, unknown>>, iots.ArrayC<iots.TupleC<[iots.Type<ColorT, ColorT, unknown>, iots.NumberC]>>]>;
    /** Reverse order of `colors` list. Only has effect when `colors` is a color list name or a color list without explicit checkpoints. */
    reverse: iots.BooleanC;
    /** Defines whether the annotation values should be normalized before assigning color based on checkpoints in `colors` (`x_normalized = (x - x_min) / (x_max - x_min)`, where `[x_min, x_max]` are either `value_domain` if provided, or the lowest and the highest value encountered in the annotation). Default is `"normalized"`. */
    mode: iots.Type<"absolute" | "normalized", "absolute" | "normalized", unknown>;
    /** Defines `x_min` and `x_max` for normalization of annotation values. Either can be `null`, meaning that minimum/maximum of the actual values will be used. Only used when `mode` is `"normalized"`. */
    value_domain: iots.TupleC<[iots.Type<number | null, number | null, unknown>, iots.Type<number | null, number | null, unknown>]>;
    /** Color to use for values below the lowest checkpoint. 'auto' means color of the lowest checkpoint. */
    underflow_color: iots.Type<"auto" | ColorT | null, "auto" | ColorT | null, unknown>;
    /** Color to use for values above the highest checkpoint. 'auto' means color of the highest checkpoint. */
    overflow_color: iots.Type<"auto" | ColorT | null, "auto" | ColorT | null, unknown>;
}>]>;
export interface ContinuousPalette extends ValueFor<typeof _ContinuousPalette> {
}
export declare const ContinuousPalette: iots.Type<ContinuousPalette>;
export declare const ContinuousPaletteDefaults: Required<ContinuousPalette>;
export declare const Palette: iots.UnionC<[iots.Type<CategoricalPalette, CategoricalPalette, unknown>, iots.Type<DiscretePalette, DiscretePalette, unknown>, iots.Type<ContinuousPalette, ContinuousPalette, unknown>]>;
export {};
