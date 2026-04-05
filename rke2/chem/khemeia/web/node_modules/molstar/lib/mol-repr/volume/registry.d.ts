/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { RepresentationRegistry, Representation, RepresentationProvider } from '../representation.js';
import { Volume } from '../../mol-model/volume.js';
export declare class VolumeRepresentationRegistry extends RepresentationRegistry<Volume, Representation.State> {
    constructor();
}
export declare namespace VolumeRepresentationRegistry {
    export const BuiltIn: {
        'direct-volume': import("./representation.js").VolumeRepresentationProvider<{
            quality: {
                isEssential: boolean;
                type: "select";
                options: readonly import("../../mol-util/param-definition.js").ParamDefinition.SelectOption<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">[];
                cycle?: boolean;
                isOptional?: boolean;
                defaultValue: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest";
                label?: string;
                description?: string;
                legend?: import("../../mol-util/legend.js").Legend;
                fieldLabels?: {
                    [name: string]: string;
                };
                isHidden?: boolean;
                shortLabel?: boolean;
                twoColumns?: boolean;
                category?: string;
                hideIf?: (currentGroup: any) => boolean;
                help?: (value: any) => {
                    description?: string;
                    legend?: import("../../mol-util/legend.js").Legend;
                };
            };
            dataType: import("../../mol-util/param-definition.js").ParamDefinition.Select<"float" | "byte" | "halfFloat">;
            ignoreLight: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            celShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            xrayShaded: import("../../mol-util/param-definition.js").ParamDefinition.Select<boolean | "inverted">;
            controlPoints: import("../../mol-util/param-definition.js").ParamDefinition.LineGraph;
            stepsPerCell: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            jumpLength: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            alpha: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            material: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                metalness: number;
                roughness: number;
                bumpiness: number;
            }>>;
            clip: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                variant: import("../../mol-util/clip.js").Clip.Variant;
                objects: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    type: /*elided*/ any;
                    invert: /*elided*/ any;
                    position: /*elided*/ any;
                    rotation: /*elided*/ any;
                    scale: /*elided*/ any;
                    transform: /*elided*/ any;
                }>[];
            }>>;
            emissive: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            density: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            instanceGranularity: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            lod: import("../../mol-util/param-definition.js").ParamDefinition.Vec3;
            cellSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            batchSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
        }, "direct-volume">;
        dot: import("./representation.js").VolumeRepresentationProvider<{
            visuals: import("../../mol-util/param-definition.js").ParamDefinition.MultiSelect<"sphere" | "point">;
            bumpFrequency: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            isoValue: import("../../mol-util/param-definition.js").ParamDefinition.Conditioned<Readonly<{
                kind: "absolute";
                absoluteValue: number;
            }> | Readonly<{
                kind: "relative";
                relativeValue: number;
            }>, import("../../mol-util/param-definition.js").ParamDefinition.Base<Readonly<{
                kind: "absolute";
                absoluteValue: number;
            }> | Readonly<{
                kind: "relative";
                relativeValue: number;
            }>>, {
                absolute: import("../../mol-util/param-definition.js").ParamDefinition.Converted<Readonly<{
                    kind: "absolute";
                    absoluteValue: number;
                }>, number>;
                relative: import("../../mol-util/param-definition.js").ParamDefinition.Converted<Readonly<{
                    kind: "relative";
                    relativeValue: number;
                }>, number>;
            }>;
            perturbPositions: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            sizeFactor: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            pointSizeAttenuation: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            pointStyle: import("../../mol-util/param-definition.js").ParamDefinition.Select<"square" | "circle" | "fuzzy">;
            alpha: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            quality: import("../../mol-util/param-definition.js").ParamDefinition.Select<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">;
            material: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                metalness: number;
                roughness: number;
                bumpiness: number;
            }>>;
            clip: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                variant: import("../../mol-util/clip.js").Clip.Variant;
                objects: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    type: /*elided*/ any;
                    invert: /*elided*/ any;
                    position: /*elided*/ any;
                    rotation: /*elided*/ any;
                    scale: /*elided*/ any;
                    transform: /*elided*/ any;
                }>[];
            }>>;
            emissive: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            density: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            instanceGranularity: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            lod: import("../../mol-util/param-definition.js").ParamDefinition.Vec3;
            cellSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            batchSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            tryUseImpostor: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            detail: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            doubleSided: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            flipSided: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            flatShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            ignoreLight: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            celShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            xrayShaded: import("../../mol-util/param-definition.js").ParamDefinition.Select<boolean | "inverted">;
            transparentBackfaces: import("../../mol-util/param-definition.js").ParamDefinition.Select<"off" | "on" | "opaque">;
            bumpAmplitude: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            interior: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                color: import("../../mol-util/color/index.js").Color;
                colorStrength: number;
                substance: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    metalness: number;
                    roughness: number;
                    bumpiness: number;
                }>;
                substanceStrength: number;
            }>>;
            solidInterior: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            clipPrimitive: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            approximate: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            alphaThickness: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            lodLevels: import("../../mol-util/param-definition.js").ParamDefinition.ObjectList<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                minDistance: number;
                maxDistance: number;
                overlap: number;
                stride: number;
                scaleBias: number;
            }>>;
        }, "dot">;
        isosurface: import("./representation.js").VolumeRepresentationProvider<{
            visuals: import("../../mol-util/param-definition.js").ParamDefinition.MultiSelect<"solid" | "wireframe">;
            bumpFrequency: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            quality: {
                isEssential: boolean;
                type: "select";
                options: readonly import("../../mol-util/param-definition.js").ParamDefinition.SelectOption<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">[];
                cycle?: boolean;
                isOptional?: boolean;
                defaultValue: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest";
                label?: string;
                description?: string;
                legend?: import("../../mol-util/legend.js").Legend;
                fieldLabels?: {
                    [name: string]: string;
                };
                isHidden?: boolean;
                shortLabel?: boolean;
                twoColumns?: boolean;
                category?: string;
                hideIf?: (currentGroup: any) => boolean;
                help?: (value: any) => {
                    description?: string;
                    legend?: import("../../mol-util/legend.js").Legend;
                };
            };
            sizeFactor: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            isoValue: import("../../mol-util/param-definition.js").ParamDefinition.Conditioned<Readonly<{
                kind: "absolute";
                absoluteValue: number;
            }> | Readonly<{
                kind: "relative";
                relativeValue: number;
            }>, import("../../mol-util/param-definition.js").ParamDefinition.Base<Readonly<{
                kind: "absolute";
                absoluteValue: number;
            }> | Readonly<{
                kind: "relative";
                relativeValue: number;
            }>>, {
                absolute: import("../../mol-util/param-definition.js").ParamDefinition.Converted<Readonly<{
                    kind: "absolute";
                    absoluteValue: number;
                }>, number>;
                relative: import("../../mol-util/param-definition.js").ParamDefinition.Converted<Readonly<{
                    kind: "relative";
                    relativeValue: number;
                }>, number>;
            }>;
            wrap: import("../../mol-util/param-definition.js").ParamDefinition.Select<"auto" | "off" | "on">;
            floodfill: import("../../mol-util/param-definition.js").ParamDefinition.Select<"inside" | "outside" | "off">;
            lineSizeAttenuation: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            alpha: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            material: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                metalness: number;
                roughness: number;
                bumpiness: number;
            }>>;
            clip: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                variant: import("../../mol-util/clip.js").Clip.Variant;
                objects: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    type: /*elided*/ any;
                    invert: /*elided*/ any;
                    position: /*elided*/ any;
                    rotation: /*elided*/ any;
                    scale: /*elided*/ any;
                    transform: /*elided*/ any;
                }>[];
            }>>;
            emissive: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            density: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            instanceGranularity: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            lod: import("../../mol-util/param-definition.js").ParamDefinition.Vec3;
            cellSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            batchSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            tryUseGpu: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            gpuDataType: import("../../mol-util/param-definition.js").ParamDefinition.Select<"float" | "byte" | "halfFloat">;
            doubleSided: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            flipSided: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            flatShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            ignoreLight: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            celShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            xrayShaded: import("../../mol-util/param-definition.js").ParamDefinition.Select<boolean | "inverted">;
            transparentBackfaces: import("../../mol-util/param-definition.js").ParamDefinition.Select<"off" | "on" | "opaque">;
            bumpAmplitude: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            interior: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                color: import("../../mol-util/color/index.js").Color;
                colorStrength: number;
                substance: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    metalness: number;
                    roughness: number;
                    bumpiness: number;
                }>;
                substanceStrength: number;
            }>>;
        }, "isosurface">;
        segment: import("./representation.js").VolumeRepresentationProvider<{
            visuals: import("../../mol-util/param-definition.js").ParamDefinition.MultiSelect<"segment">;
            bumpFrequency: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            quality: {
                isEssential: boolean;
                type: "select";
                options: readonly import("../../mol-util/param-definition.js").ParamDefinition.SelectOption<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">[];
                cycle?: boolean;
                isOptional?: boolean;
                defaultValue: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest";
                label?: string;
                description?: string;
                legend?: import("../../mol-util/legend.js").Legend;
                fieldLabels?: {
                    [name: string]: string;
                };
                isHidden?: boolean;
                shortLabel?: boolean;
                twoColumns?: boolean;
                category?: string;
                hideIf?: (currentGroup: any) => boolean;
                help?: (value: any) => {
                    description?: string;
                    legend?: import("../../mol-util/legend.js").Legend;
                };
            };
            tryUseGpu: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            segments: import("../../mol-util/param-definition.js").ParamDefinition.Converted<number[], string[]>;
            doubleSided: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            flipSided: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            flatShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            ignoreLight: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            celShaded: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            xrayShaded: import("../../mol-util/param-definition.js").ParamDefinition.Select<boolean | "inverted">;
            transparentBackfaces: import("../../mol-util/param-definition.js").ParamDefinition.Select<"off" | "on" | "opaque">;
            bumpAmplitude: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            interior: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                color: import("../../mol-util/color/index.js").Color;
                colorStrength: number;
                substance: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    metalness: number;
                    roughness: number;
                    bumpiness: number;
                }>;
                substanceStrength: number;
            }>>;
            alpha: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            material: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                metalness: number;
                roughness: number;
                bumpiness: number;
            }>>;
            clip: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                variant: import("../../mol-util/clip.js").Clip.Variant;
                objects: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    type: /*elided*/ any;
                    invert: /*elided*/ any;
                    position: /*elided*/ any;
                    rotation: /*elided*/ any;
                    scale: /*elided*/ any;
                    transform: /*elided*/ any;
                }>[];
            }>>;
            emissive: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            density: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            instanceGranularity: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            lod: import("../../mol-util/param-definition.js").ParamDefinition.Vec3;
            cellSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            batchSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
        }, "segment">;
        slice: import("./representation.js").VolumeRepresentationProvider<{
            quality: {
                isEssential: boolean;
                type: "select";
                options: readonly import("../../mol-util/param-definition.js").ParamDefinition.SelectOption<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">[];
                cycle?: boolean;
                isOptional?: boolean;
                defaultValue: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest";
                label?: string;
                description?: string;
                legend?: import("../../mol-util/legend.js").Legend;
                fieldLabels?: {
                    [name: string]: string;
                };
                isHidden?: boolean;
                shortLabel?: boolean;
                twoColumns?: boolean;
                category?: string;
                hideIf?: (currentGroup: any) => boolean;
                help?: (value: any) => {
                    description?: string;
                    legend?: import("../../mol-util/legend.js").Legend;
                };
            };
            dimension: import("../../mol-util/param-definition.js").ParamDefinition.Mapped<import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<number, "x"> | import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<number, "y"> | import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<number, "z"> | import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<number, "relativeX"> | import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<number, "relativeY"> | import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<number, "relativeZ">>;
            isoValue: import("../../mol-util/param-definition.js").ParamDefinition.Conditioned<Readonly<{
                kind: "absolute";
                absoluteValue: number;
            }> | Readonly<{
                kind: "relative";
                relativeValue: number;
            }>, import("../../mol-util/param-definition.js").ParamDefinition.Base<Readonly<{
                kind: "absolute";
                absoluteValue: number;
            }> | Readonly<{
                kind: "relative";
                relativeValue: number;
            }>>, {
                absolute: import("../../mol-util/param-definition.js").ParamDefinition.Converted<Readonly<{
                    kind: "absolute";
                    absoluteValue: number;
                }>, number>;
                relative: import("../../mol-util/param-definition.js").ParamDefinition.Converted<Readonly<{
                    kind: "relative";
                    relativeValue: number;
                }>, number>;
            }>;
            mode: import("../../mol-util/param-definition.js").ParamDefinition.Select<"grid" | "frame" | "plane">;
            offset: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            axis: import("../../mol-util/param-definition.js").ParamDefinition.Select<"a" | "b" | "c">;
            rotation: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                axis: import("../../mol-math/linear-algebra.js").Vec3;
                angle: number;
            }>>;
            plane: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                point: import("../../mol-math/linear-algebra.js").Vec3;
                normal: import("../../mol-math/linear-algebra.js").Vec3;
            }>>;
            interpolation: import("../../mol-util/param-definition.js").ParamDefinition.Select<"nearest" | "catmulrom" | "mitchell" | "bspline">;
            alpha: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            material: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                metalness: number;
                roughness: number;
                bumpiness: number;
            }>>;
            clip: import("../../mol-util/param-definition.js").ParamDefinition.Group<import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                variant: import("../../mol-util/clip.js").Clip.Variant;
                objects: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
                    type: /*elided*/ any;
                    invert: /*elided*/ any;
                    position: /*elided*/ any;
                    rotation: /*elided*/ any;
                    scale: /*elided*/ any;
                    transform: /*elided*/ any;
                }>[];
            }>>;
            emissive: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            density: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            instanceGranularity: import("../../mol-util/param-definition.js").ParamDefinition.BooleanParam;
            lod: import("../../mol-util/param-definition.js").ParamDefinition.Vec3;
            cellSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
            batchSize: import("../../mol-util/param-definition.js").ParamDefinition.Numeric;
        }, "slice">;
    };
    type _BuiltIn = typeof BuiltIn;
    export type BuiltIn = keyof _BuiltIn;
    export type BuiltInParams<T extends BuiltIn> = Partial<RepresentationProvider.ParamValues<_BuiltIn[T]>>;
    export {};
}
