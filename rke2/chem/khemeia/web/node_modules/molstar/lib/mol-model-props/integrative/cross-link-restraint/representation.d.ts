/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { RepresentationContext, RepresentationParamsGetter } from '../../../mol-repr/representation.js';
import { ThemeRegistryContext } from '../../../mol-theme/theme.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { Structure } from '../../../mol-model/structure.js';
import { ComplexVisual } from '../../../mol-repr/structure/complex-visual.js';
import { StructureRepresentation, StructureRepresentationProvider } from '../../../mol-repr/structure/representation.js';
import { CrossLinkRestraint } from './property.js';
export declare const CrossLinkRestraintCylinderParams: {
    sizeFactor: PD.Numeric;
    linkScale: PD.Numeric;
    linkSpacing: PD.Numeric;
    linkCap: PD.BooleanParam;
    aromaticScale: PD.Numeric;
    aromaticSpacing: PD.Numeric;
    aromaticDashCount: PD.Numeric;
    dashCount: PD.Numeric;
    dashScale: PD.Numeric;
    dashCap: PD.BooleanParam;
    stubCap: PD.BooleanParam;
    radialSegments: PD.Numeric;
    colorMode: PD.Select<"default" | "interpolate">;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    doubleSided: PD.BooleanParam;
    flipSided: PD.BooleanParam;
    flatShaded: PD.BooleanParam;
    ignoreLight: PD.BooleanParam;
    celShaded: PD.BooleanParam;
    xrayShaded: PD.Select<boolean | "inverted">;
    transparentBackfaces: PD.Select<"off" | "on" | "opaque">;
    bumpFrequency: PD.Numeric;
    bumpAmplitude: PD.Numeric;
    interior: PD.Group<PD.Normalize<{
        color: import("../../../mol-util/color/index.js").Color;
        colorStrength: number;
        substance: PD.Normalize<{
            metalness: number;
            roughness: number;
            bumpiness: number;
        }>;
        substanceStrength: number;
    }>>;
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
export type CrossLinkRestraintCylinderParams = typeof CrossLinkRestraintCylinderParams;
export declare function CrossLinkRestraintVisual(materialId: number): ComplexVisual<CrossLinkRestraintCylinderParams>;
export declare const CrossLinkRestraintParams: {
    sizeFactor: PD.Numeric;
    linkScale: PD.Numeric;
    linkSpacing: PD.Numeric;
    linkCap: PD.BooleanParam;
    aromaticScale: PD.Numeric;
    aromaticSpacing: PD.Numeric;
    aromaticDashCount: PD.Numeric;
    dashCount: PD.Numeric;
    dashScale: PD.Numeric;
    dashCap: PD.BooleanParam;
    stubCap: PD.BooleanParam;
    radialSegments: PD.Numeric;
    colorMode: PD.Select<"default" | "interpolate">;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    doubleSided: PD.BooleanParam;
    flipSided: PD.BooleanParam;
    flatShaded: PD.BooleanParam;
    ignoreLight: PD.BooleanParam;
    celShaded: PD.BooleanParam;
    xrayShaded: PD.Select<boolean | "inverted">;
    transparentBackfaces: PD.Select<"off" | "on" | "opaque">;
    bumpFrequency: PD.Numeric;
    bumpAmplitude: PD.Numeric;
    interior: PD.Group<PD.Normalize<{
        color: import("../../../mol-util/color/index.js").Color;
        colorStrength: number;
        substance: PD.Normalize<{
            metalness: number;
            roughness: number;
            bumpiness: number;
        }>;
        substanceStrength: number;
    }>>;
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
export type CrossLinkRestraintParams = typeof CrossLinkRestraintParams;
export declare function getCrossLinkRestraintParams(ctx: ThemeRegistryContext, structure: Structure): {
    sizeFactor: PD.Numeric;
    linkScale: PD.Numeric;
    linkSpacing: PD.Numeric;
    linkCap: PD.BooleanParam;
    aromaticScale: PD.Numeric;
    aromaticSpacing: PD.Numeric;
    aromaticDashCount: PD.Numeric;
    dashCount: PD.Numeric;
    dashScale: PD.Numeric;
    dashCap: PD.BooleanParam;
    stubCap: PD.BooleanParam;
    radialSegments: PD.Numeric;
    colorMode: PD.Select<"default" | "interpolate">;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    doubleSided: PD.BooleanParam;
    flipSided: PD.BooleanParam;
    flatShaded: PD.BooleanParam;
    ignoreLight: PD.BooleanParam;
    celShaded: PD.BooleanParam;
    xrayShaded: PD.Select<boolean | "inverted">;
    transparentBackfaces: PD.Select<"off" | "on" | "opaque">;
    bumpFrequency: PD.Numeric;
    bumpAmplitude: PD.Numeric;
    interior: PD.Group<PD.Normalize<{
        color: import("../../../mol-util/color/index.js").Color;
        colorStrength: number;
        substance: PD.Normalize<{
            metalness: number;
            roughness: number;
            bumpiness: number;
        }>;
        substanceStrength: number;
    }>>;
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
export type CrossLinkRestraintRepresentation = StructureRepresentation<CrossLinkRestraintParams>;
export declare function CrossLinkRestraintRepresentation(ctx: RepresentationContext, getParams: RepresentationParamsGetter<Structure, CrossLinkRestraintParams>): CrossLinkRestraintRepresentation;
export declare const CrossLinkRestraintRepresentationProvider: StructureRepresentationProvider<{
    sizeFactor: PD.Numeric;
    linkScale: PD.Numeric;
    linkSpacing: PD.Numeric;
    linkCap: PD.BooleanParam;
    aromaticScale: PD.Numeric;
    aromaticSpacing: PD.Numeric;
    aromaticDashCount: PD.Numeric;
    dashCount: PD.Numeric;
    dashScale: PD.Numeric;
    dashCap: PD.BooleanParam;
    stubCap: PD.BooleanParam;
    radialSegments: PD.Numeric;
    colorMode: PD.Select<"default" | "interpolate">;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    includeParent: PD.BooleanParam;
    doubleSided: PD.BooleanParam;
    flipSided: PD.BooleanParam;
    flatShaded: PD.BooleanParam;
    ignoreLight: PD.BooleanParam;
    celShaded: PD.BooleanParam;
    xrayShaded: PD.Select<boolean | "inverted">;
    transparentBackfaces: PD.Select<"off" | "on" | "opaque">;
    bumpFrequency: PD.Numeric;
    bumpAmplitude: PD.Numeric;
    interior: PD.Group<PD.Normalize<{
        color: import("../../../mol-util/color/index.js").Color;
        colorStrength: number;
        substance: PD.Normalize<{
            metalness: number;
            roughness: number;
            bumpiness: number;
        }>;
        substanceStrength: number;
    }>>;
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
}, CrossLinkRestraint.Tag>;
