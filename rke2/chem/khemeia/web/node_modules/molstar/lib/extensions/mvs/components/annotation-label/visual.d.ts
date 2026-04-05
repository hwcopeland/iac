/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { ComplexVisual } from '../../../../mol-repr/structure/complex-visual.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
/** Parameter definition for "label-text" visual in "MVS Annotation Label" representation */
export type MVSAnnotationLabelTextParams = typeof MVSAnnotationLabelTextParams;
export declare const MVSAnnotationLabelTextParams: {
    borderColor: {
        defaultValue: import("../../../../mol-util/color/index.js").Color;
        type: "color";
        isExpanded?: boolean;
        isOptional?: boolean;
        label?: string;
        description?: string;
        legend?: import("../../../../mol-util/legend.js").Legend;
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
            legend?: import("../../../../mol-util/legend.js").Legend;
        };
    };
    alpha: PD.Numeric;
    clip: PD.Group<PD.Normalize<{
        variant: import("../../../../mol-util/clip.js").Clip.Variant;
        objects: PD.Normalize<{
            type: /*elided*/ any;
            invert: /*elided*/ any;
            position: /*elided*/ any;
            rotation: /*elided*/ any;
            scale: /*elided*/ any;
            transform: /*elided*/ any;
        }>[];
    }>>;
    backgroundColor: PD.Color;
    fontFamily: PD.Select<import("../../../../mol-geo/geometry/text/font-atlas.js").FontFamily>;
    fontStyle: PD.Select<import("../../../../mol-geo/geometry/text/font-atlas.js").FontStyle>;
    fontVariant: PD.Select<import("../../../../mol-geo/geometry/text/font-atlas.js").FontVariant>;
    fontWeight: PD.Select<import("../../../../mol-geo/geometry/text/font-atlas.js").FontWeight>;
    background: PD.BooleanParam;
    borderWidth: PD.Numeric;
    emissive: PD.Numeric;
    batchSize: PD.Numeric;
    material: PD.Group<PD.Normalize<{
        metalness: number;
        roughness: number;
        bumpiness: number;
    }>>;
    instanceGranularity: PD.BooleanParam;
    quality: PD.Select<"auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest">;
    density: PD.Numeric;
    lod: PD.Vec3;
    cellSize: PD.Numeric;
    sizeFactor: PD.Numeric;
    fontQuality: PD.Select<number>;
    offsetX: PD.Numeric;
    offsetY: PD.Numeric;
    offsetZ: PD.Numeric;
    backgroundMargin: PD.Numeric;
    backgroundOpacity: PD.Numeric;
    tether: PD.BooleanParam;
    tetherLength: PD.Numeric;
    tetherBaseWidth: PD.Numeric;
    attachment: PD.Select<"bottom-left" | "bottom-center" | "bottom-right" | "middle-left" | "middle-center" | "middle-right" | "top-left" | "top-center" | "top-right">;
    ignoreHydrogens: PD.BooleanParam;
    ignoreHydrogensVariant: PD.Select<"all" | "non-polar">;
    includeParent: PD.BooleanParam;
    unitKinds: PD.MultiSelect<"spheres" | "gaussians" | "atomic">;
    annotationId: PD.Text<string>;
    fieldName: PD.Text<string>;
    textFormat: PD.Text<string>;
    groupByFields: PD.ObjectList<PD.Normalize<{
        fieldName: string;
    }>>;
};
/** Parameter values for "label-text" visual in "MVS Annotation Label" representation */
export type MVSAnnotationLabelTextProps = PD.Values<MVSAnnotationLabelTextParams>;
/** Create "label-text" visual for "MVS Annotation Label" representation */
export declare function MVSAnnotationLabelTextVisual(materialId: number): ComplexVisual<MVSAnnotationLabelTextParams>;
