/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { PluginConfigItem } from '../../mol-plugin/config.js';
import { PluginContext } from '../../mol-plugin/context.js';
import { StateTransformer, StateAction, StateObject, StateTransform, StateObjectRef } from '../../mol-state/index.js';
import { StructureRepresentationPresetProvider } from '../../mol-plugin-state/builder/structure/representation-preset.js';
export declare const AssemblySymmetry: StateTransformer<any, any, any>;
export declare const InitAssemblySymmetry3D: StateAction<PluginStateObject.Molecule.Structure, void, PD.Normalize<{
    serverType: "rcsb" | "pdbe";
    serverUrl: string;
}>>;
export { AssemblySymmetry3D };
type AssemblySymmetry3D = typeof AssemblySymmetry3D;
declare const AssemblySymmetry3D: StateTransformer<PluginStateObject.Molecule.Structure, PluginStateObject.Shape.Representation3D, PD.Normalize<{
    visuals: ("axes" | "cage")[];
    cageColor: import("../../mol-util/color/index.js").Color;
    scale: number;
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
    axesColor: PD.NamedParams<PD.Normalize<{
        colorValue: /*elided*/ any;
    }>, "uniform"> | PD.NamedParams<PD.Normalize<unknown>, "byOrder">;
}>>;
export declare const AssemblySymmetryPreset: StructureRepresentationPresetProvider<{
    serverType: "rcsb" | "pdbe";
    serverUrl: string;
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
        polymer: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>> | undefined;
    } | {
        all: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>> | undefined;
        branched: undefined;
    } | undefined;
    representations: {
        assemblySymmetry: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>>;
    } | {
        assemblySymmetry: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>>;
        polymer: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure.Representation3D, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>>;
    } | {
        assemblySymmetry: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>>;
        all: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Molecule.Structure.Representation3D, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>>;
    };
}>;
export declare function tryCreateAssemblySymmetry(plugin: PluginContext, structure: StateObjectRef<PluginStateObject.Molecule.Structure>, params?: StateTransformer.Params<AssemblySymmetry3D>, initialState?: Partial<StateTransform.State>): Promise<import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, StateTransformer<StateObject<any, StateObject.Type<any>>, StateObject<any, StateObject.Type<any>>, any>>>;
export declare const AssemblySymmetryConfig: {
    DefaultServerType: PluginConfigItem<"rcsb" | "pdbe">;
    DefaultServerUrl: PluginConfigItem<string>;
    ApplyColors: PluginConfigItem<boolean>;
};
export declare function getAssemblySymmetryConfig(plugin: PluginContext): {
    [key in keyof typeof AssemblySymmetryConfig]: NonNullable<typeof AssemblySymmetryConfig[key]['defaultValue']>;
};
