/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Michal Malý <michal.maly@ibt.cas.cz>
 * @author Jiří Černý <jiri.cerny@ibt.cas.cz>
 */
import { DnatcoTypes } from '../types.js';
import { StructureRepresentationPresetProvider } from '../../../mol-plugin-state/builder/structure/representation-preset.js';
export declare const NtCTubePreset: StructureRepresentationPresetProvider<{
    ignoreHydrogens: boolean | undefined;
    ignoreHydrogensVariant: "all" | "non-polar" | undefined;
    ignoreLight: boolean | undefined;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest" | undefined;
    theme: import("../../../mol-util/param-definition.js").ParamDefinition.Normalize<{
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
        tube: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    } | {
        tube: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        polymer: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    } | {
        tube: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        all: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        branched: undefined;
    };
    representations: {
        tubeRepr: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    } | {
        tubeRepr: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        polymer: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>>;
    } | {
        tubeRepr: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        all: import("../../../mol-state/index.js").StateObjectSelector<import("../../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../../mol-state/index.js").StateTransformer<import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, import("../../../mol-state/index.js").StateObject<any, import("../../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
}>;
export declare function NtCTubeSegmentLabel(step: DnatcoTypes.Step): string;
