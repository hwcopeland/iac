/**
 * Copyright (c) 2019-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { StructureRepresentation, StructureRepresentationProvider } from '../representation.js';
import { RepresentationParamsGetter, RepresentationContext } from '../../../mol-repr/representation.js';
import { ThemeRegistryContext } from '../../../mol-theme/theme.js';
import { Structure } from '../../../mol-model/structure.js';
export declare const LabelParams: {
    visuals: PD.MultiSelect<"label-text">;
    background: PD.BooleanParam;
    backgroundMargin: PD.Numeric;
    backgroundColor: PD.Color;
    backgroundOpacity: PD.Numeric;
    borderWidth: PD.Numeric;
    level: PD.Select<"element" | "residue" | "chain">;
    ignoreHydrogens: PD.BooleanParam;
    ignoreHydrogensVariant: PD.Select<"all" | "non-polar">;
    chainScale: PD.Numeric;
    residueScale: PD.Numeric;
    elementScale: PD.Numeric;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    sizeFactor: PD.Numeric;
    borderColor: PD.Color;
    offsetX: PD.Numeric;
    offsetY: PD.Numeric;
    offsetZ: PD.Numeric;
    tether: PD.BooleanParam;
    tetherLength: PD.Numeric;
    tetherBaseWidth: PD.Numeric;
    attachment: PD.Select<"bottom-left" | "bottom-center" | "bottom-right" | "middle-left" | "middle-center" | "middle-right" | "top-left" | "top-center" | "top-right">;
    fontFamily: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontFamily>;
    fontQuality: PD.Select<number>;
    fontStyle: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontStyle>;
    fontVariant: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontVariant>;
    fontWeight: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontWeight>;
    alpha: PD.Numeric;
    quality: PD.Select<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">;
    material: PD.Group<PD.Normalize<{
        metalness: number;
        roughness: number;
        bumpiness: number;
    }>>;
    clip: PD.Group<PD.Normalize<{
        variant: import("../../../mol-util/clip.js").Clip.Variant;
        objects: PD.Normalize<{
            type: /*elided*/ any;
            invert: /*elided*/ any;
            position: /*elided*/ any;
            rotation: /*elided*/ any;
            scale: /*elided*/ any;
            transform: /*elided*/ any;
        }>[];
    }>>;
    emissive: PD.Numeric;
    density: PD.Numeric;
    instanceGranularity: PD.BooleanParam;
    lod: PD.Vec3;
    cellSize: PD.Numeric;
    batchSize: PD.Numeric;
};
export type LabelParams = typeof LabelParams;
export declare function getLabelParams(ctx: ThemeRegistryContext, structure: Structure): {
    visuals: PD.MultiSelect<"label-text">;
    background: PD.BooleanParam;
    backgroundMargin: PD.Numeric;
    backgroundColor: PD.Color;
    backgroundOpacity: PD.Numeric;
    borderWidth: PD.Numeric;
    level: PD.Select<"element" | "residue" | "chain">;
    ignoreHydrogens: PD.BooleanParam;
    ignoreHydrogensVariant: PD.Select<"all" | "non-polar">;
    chainScale: PD.Numeric;
    residueScale: PD.Numeric;
    elementScale: PD.Numeric;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    sizeFactor: PD.Numeric;
    borderColor: PD.Color;
    offsetX: PD.Numeric;
    offsetY: PD.Numeric;
    offsetZ: PD.Numeric;
    tether: PD.BooleanParam;
    tetherLength: PD.Numeric;
    tetherBaseWidth: PD.Numeric;
    attachment: PD.Select<"bottom-left" | "bottom-center" | "bottom-right" | "middle-left" | "middle-center" | "middle-right" | "top-left" | "top-center" | "top-right">;
    fontFamily: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontFamily>;
    fontQuality: PD.Select<number>;
    fontStyle: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontStyle>;
    fontVariant: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontVariant>;
    fontWeight: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontWeight>;
    alpha: PD.Numeric;
    quality: PD.Select<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">;
    material: PD.Group<PD.Normalize<{
        metalness: number;
        roughness: number;
        bumpiness: number;
    }>>;
    clip: PD.Group<PD.Normalize<{
        variant: import("../../../mol-util/clip.js").Clip.Variant;
        objects: PD.Normalize<{
            type: /*elided*/ any;
            invert: /*elided*/ any;
            position: /*elided*/ any;
            rotation: /*elided*/ any;
            scale: /*elided*/ any;
            transform: /*elided*/ any;
        }>[];
    }>>;
    emissive: PD.Numeric;
    density: PD.Numeric;
    instanceGranularity: PD.BooleanParam;
    lod: PD.Vec3;
    cellSize: PD.Numeric;
    batchSize: PD.Numeric;
};
export type LabelRepresentation = StructureRepresentation<LabelParams>;
export declare function LabelRepresentation(ctx: RepresentationContext, getParams: RepresentationParamsGetter<Structure, LabelParams>): LabelRepresentation;
export declare const LabelRepresentationProvider: StructureRepresentationProvider<{
    visuals: PD.MultiSelect<"label-text">;
    background: PD.BooleanParam;
    backgroundMargin: PD.Numeric;
    backgroundColor: PD.Color;
    backgroundOpacity: PD.Numeric;
    borderWidth: PD.Numeric;
    level: PD.Select<"element" | "residue" | "chain">;
    ignoreHydrogens: PD.BooleanParam;
    ignoreHydrogensVariant: PD.Select<"all" | "non-polar">;
    chainScale: PD.Numeric;
    residueScale: PD.Numeric;
    elementScale: PD.Numeric;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    sizeFactor: PD.Numeric;
    borderColor: PD.Color;
    offsetX: PD.Numeric;
    offsetY: PD.Numeric;
    offsetZ: PD.Numeric;
    tether: PD.BooleanParam;
    tetherLength: PD.Numeric;
    tetherBaseWidth: PD.Numeric;
    attachment: PD.Select<"bottom-left" | "bottom-center" | "bottom-right" | "middle-left" | "middle-center" | "middle-right" | "top-left" | "top-center" | "top-right">;
    fontFamily: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontFamily>;
    fontQuality: PD.Select<number>;
    fontStyle: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontStyle>;
    fontVariant: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontVariant>;
    fontWeight: PD.Select<import("../../../mol-geo/geometry/text/font-atlas.js").FontWeight>;
    alpha: PD.Numeric;
    quality: PD.Select<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">;
    material: PD.Group<PD.Normalize<{
        metalness: number;
        roughness: number;
        bumpiness: number;
    }>>;
    clip: PD.Group<PD.Normalize<{
        variant: import("../../../mol-util/clip.js").Clip.Variant;
        objects: PD.Normalize<{
            type: /*elided*/ any;
            invert: /*elided*/ any;
            position: /*elided*/ any;
            rotation: /*elided*/ any;
            scale: /*elided*/ any;
            transform: /*elided*/ any;
        }>[];
    }>>;
    emissive: PD.Numeric;
    density: PD.Numeric;
    instanceGranularity: PD.BooleanParam;
    lod: PD.Vec3;
    cellSize: PD.Numeric;
    batchSize: PD.Numeric;
}, "label">;
