/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
export declare const TopologyFormatCategory = "Topology";
export { PsfProvider };
declare const PsfProvider: {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Format.Psf, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Data.String, import("../objects.js").PluginStateObject.Format.Psf, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        topology: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Format.Psf, import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
    }>;
};
type PsfProvider = typeof PsfProvider;
export { PrmtopProvider };
declare const PrmtopProvider: {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Format.Prmtop, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Data.String, import("../objects.js").PluginStateObject.Format.Prmtop, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        topology: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Format.Prmtop, import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
    }>;
};
type PrmtopProvider = typeof PrmtopProvider;
export { TopProvider };
declare const TopProvider: {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Format.Top, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Data.String, import("../objects.js").PluginStateObject.Format.Top, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        topology: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Format.Top, import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
    }>;
};
type TopProvider = typeof TopProvider;
export type TopologyProvider = PsfProvider;
export declare const BuiltInTopologyFormats: readonly [readonly ["psf", {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Format.Psf, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Data.String, import("../objects.js").PluginStateObject.Format.Psf, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        topology: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Format.Psf, import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
    }>;
}], readonly ["prmtop", {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Format.Prmtop, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Data.String, import("../objects.js").PluginStateObject.Format.Prmtop, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        topology: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Format.Prmtop, import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
    }>;
}], readonly ["top", {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Format.Top, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Data.String, import("../objects.js").PluginStateObject.Format.Top, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        topology: import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-state/index.js").StateTransformer<import("../objects.js").PluginStateObject.Format.Top, import("../objects.js").PluginStateObject.Molecule.Topology, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
    }>;
}]];
export type BuiltInTopologyFormat = (typeof BuiltInTopologyFormats)[number][0];
