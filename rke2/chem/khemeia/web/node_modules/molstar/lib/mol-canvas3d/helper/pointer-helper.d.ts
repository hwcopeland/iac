/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Scene } from '../../mol-gl/scene.js';
import { Vec3 } from '../../mol-math/linear-algebra.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Camera, ICamera } from '../camera.js';
import { Ray3D } from '../../mol-math/geometry/primitives/ray3d.js';
export declare const PointerHelperParams: {
    enabled: PD.Select<"off" | "on">;
    ignoreLight: {
        defaultValue: boolean;
        type: "boolean";
        isOptional?: boolean;
        label?: string;
        description?: string;
        legend?: import("../../mol-util/legend.js").Legend;
        fieldLabels?: {
            [name: string]: string;
        };
        isHidden?: boolean;
        shortLabel?: boolean;
        twoColumns?: boolean;
        isEssential?: boolean;
        category?: string;
        hideIf?: (currentGroup: any) => boolean;
        help?: (value: any) => {
            description?: string;
            legend?: import("../../mol-util/legend.js").Legend;
        };
    };
    color: PD.Color;
    hitColor: PD.Color;
    doubleSided: PD.BooleanParam;
    flipSided: PD.BooleanParam;
    flatShaded: PD.BooleanParam;
    celShaded: PD.BooleanParam;
    xrayShaded: PD.Select<boolean | "inverted">;
    transparentBackfaces: PD.Select<"off" | "on" | "opaque">;
    bumpFrequency: PD.Numeric;
    bumpAmplitude: PD.Numeric;
    interior: PD.Group<PD.Normalize<{
        color: import("../../mol-util/color/index.js").Color;
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
        variant: import("../../mol-util/clip.js").Clip.Variant;
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
export type PointerHelperParams = typeof PointerHelperParams;
export type PointerHelperProps = PD.Values<PointerHelperParams>;
export declare class PointerHelper {
    readonly scene: Scene;
    readonly camera: Camera;
    readonly props: PointerHelperProps;
    pixelScale: number;
    private renderObject;
    private shape;
    private modelScale;
    private pointers;
    private points;
    private hit;
    setProps(props: Partial<PointerHelperProps>): void;
    ensureEnabled(): void;
    get isEnabled(): boolean;
    setCamera(camera: ICamera): void;
    update(pointers: Ray3D[], points: Vec3[], hit: Vec3 | undefined): void;
    private getData;
    constructor(webgl: WebGLContext, props?: Partial<PointerHelperProps>);
}
export declare enum PointerHelperGroup {
    None = 0,
    Hit = 1
}
