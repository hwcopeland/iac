/**
 * Copyright (c) 2020-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { StructureRepresentationPresetProvider } from '../../mol-plugin-state/builder/structure/representation-preset.js';
import { StructureRef } from '../../mol-plugin-state/manager/structure/hierarchy-state.js';
import { PluginUIComponent } from '../../mol-plugin-ui/base.js';
export declare const StructurePreset: StructureRepresentationPresetProvider<{
    ignoreHydrogens: boolean | undefined;
    ignoreHydrogensVariant: "all" | "non-polar" | undefined;
    ignoreLight: boolean | undefined;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest" | undefined;
    theme: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
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
        ligand: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        polymer: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    };
    representations: {
        ligand: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
        polymer: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
}>;
export declare const IllustrativePreset: StructureRepresentationPresetProvider<{
    ignoreHydrogens: boolean | undefined;
    ignoreHydrogensVariant: "all" | "non-polar" | undefined;
    ignoreLight: boolean | undefined;
    quality: "auto" | "medium" | "high" | "low" | "custom" | "highest" | "higher" | "lower" | "lowest" | undefined;
    theme: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
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
        ligand: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
        polymer: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>> | undefined;
    };
    representations: {
        ligand: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
        polymer: import("../../mol-state/index.js").StateObjectSelector<import("../../mol-plugin-state/objects.js").PluginStateObject.Molecule.Structure.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>;
    };
}>;
export declare const ShowButtons: import("../../mol-plugin/config.js").PluginConfigItem<boolean>;
export declare class ViewportComponent extends PluginUIComponent {
    _set(structures: readonly StructureRef[], preset: StructureRepresentationPresetProvider): Promise<void>;
    set: (preset: StructureRepresentationPresetProvider) => Promise<void>;
    structurePreset: () => Promise<void>;
    illustrativePreset: () => Promise<void>;
    surfacePreset: () => Promise<void>;
    pocketPreset: () => Promise<void>;
    interactionsPreset: () => Promise<void>;
    get showButtons(): boolean | undefined;
    render(): import("react/jsx-runtime").JSX.Element;
}
