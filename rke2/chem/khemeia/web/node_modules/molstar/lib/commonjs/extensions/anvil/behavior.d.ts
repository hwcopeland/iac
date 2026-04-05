/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Sebastian Bittrich <sebastian.bittrich@rcsb.org>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { StructureRepresentationPresetProvider } from '../../mol-plugin-state/builder/structure/representation-preset.js';
import { StateObjectRef, StateTransformer, StateTransform } from '../../mol-state/index.js';
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { PluginContext } from '../../mol-plugin/context.js';
import { StructureSelectionQuery } from '../../mol-plugin-state/helpers/structure-selection-query.js';
export declare const ANVILMembraneOrientation: StateTransformer<any, any, any>;
export declare const isTransmembrane: StructureSelectionQuery;
export { MembraneOrientation3D };
type MembraneOrientation3D = typeof MembraneOrientation3D;
declare const MembraneOrientation3D: StateTransformer<PluginStateObject.Molecule.Structure, PluginStateObject.Shape.Representation3D, PD.Normalize<{
    visuals: ("bilayer-planes" | "bilayer-rims")[];
    lineSizeAttenuation: boolean;
    linesSize: number;
    dashedLines: boolean;
    color: import("../../mol-util/color/index.js").Color;
    radiusFactor: number;
    sizeFactor: number;
    alpha: number;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest";
    material: PD.Normalize<{
        metalness: number;
        roughness: number;
        bumpiness: number;
    }>;
    clip: PD.Normalize<{
        variant: /*elided*/ any;
        objects: /*elided*/ any;
    }>;
    emissive: number;
    density: number;
    instanceGranularity: boolean;
    lod: import("../../mol-math/linear-algebra.js").Vec3;
    cellSize: number;
    batchSize: number;
    sectorOpacity: number;
    doubleSided: boolean;
    flipSided: boolean;
    flatShaded: boolean;
    ignoreLight: boolean;
    celShaded: boolean;
    xrayShaded: boolean | "inverted";
    transparentBackfaces: "off" | "on" | "opaque";
    bumpFrequency: number;
    bumpAmplitude: number;
    interior: PD.Normalize<{
        color: /*elided*/ any;
        colorStrength: /*elided*/ any;
        substance: /*elided*/ any;
        substanceStrength: /*elided*/ any;
    }>;
}>>;
export declare const MembraneOrientationPreset: StructureRepresentationPresetProvider<{
    ignoreHydrogens: boolean | undefined;
    ignoreHydrogensVariant: "all" | "non-polar" | undefined;
    ignoreLight: boolean | undefined;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest" | undefined;
    theme: PD.Normalize<{
        globalName: /*elided*/ any;
        globalColorParams: /*elided*/ any;
        carbonColor: /*elided*/ any;
        symmetryColor: /*elided*/ any;
        symmetryColorParams: /*elided*/ any;
        focus: /*elided*/ any;
    }> | undefined;
}, {
    components?: undefined;
    representations?: undefined;
} | {
    components: {
        polymer: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    } | {
        all: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        branched: undefined;
    } | undefined;
    representations: {
        membraneOrientation: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
    } | {
        membraneOrientation: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
        polymer: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
    } | {
        membraneOrientation: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
        all: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
}>;
export declare function tryCreateMembraneOrientation(plugin: PluginContext, structure: StateObjectRef<PluginStateObject.Molecule.Structure>, params?: StateTransformer.Params<MembraneOrientation3D>, initialState?: Partial<StateTransform.State>): Promise<import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
