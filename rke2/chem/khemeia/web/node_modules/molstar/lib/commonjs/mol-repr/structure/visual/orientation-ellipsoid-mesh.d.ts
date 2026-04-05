/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { UnitsVisual } from '../../../mol-repr/structure/units-visual.js';
import { VisualContext } from '../../../mol-repr/visual.js';
import { Unit, Structure } from '../../../mol-model/structure.js';
import { Theme } from '../../../mol-theme/theme.js';
import { Mesh } from '../../../mol-geo/geometry/mesh/mesh.js';
export declare const OrientationEllipsoidMeshParams: {
    sizeFactor: PD.Numeric;
    detail: PD.Numeric;
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
export type OrientationEllipsoidMeshParams = typeof OrientationEllipsoidMeshParams;
export declare function OrientationEllipsoidMeshVisual(materialId: number): UnitsVisual<OrientationEllipsoidMeshParams>;
export interface OrientationEllipsoidMeshProps {
    detail: number;
    sizeFactor: number;
}
export declare function createOrientationEllipsoidMesh(ctx: VisualContext, unit: Unit, structure: Structure, theme: Theme, props: OrientationEllipsoidMeshProps, mesh?: Mesh): Mesh;
