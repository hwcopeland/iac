/**
 * Copyright (c) 2018-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Ludovic Autin <ludovic.autin@gmail.com>
 */
export declare const CoordinatesFormatCategory = "Coordinates";
export { DcdProvider };
declare const DcdProvider: {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
};
type DcdProvider = typeof DcdProvider;
export { XtcProvider };
declare const XtcProvider: {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
};
type XtcProvider = typeof XtcProvider;
export { TrrProvider };
declare const TrrProvider: {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
};
type TrrProvider = typeof TrrProvider;
export { NctrajProvider };
declare const NctrajProvider: {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
};
type NctrajProvider = typeof NctrajProvider;
export { LammpsTrajectoryProvider };
declare const LammpsTrajectoryProvider: {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
};
type LammpsTrajectoryProvider = typeof LammpsTrajectoryProvider;
export type CoordinatesProvider = DcdProvider | XtcProvider | TrrProvider | LammpsTrajectoryProvider;
export declare const BuiltInCoordinatesFormats: readonly [readonly ["dcd", {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}], readonly ["xtc", {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}], readonly ["trr", {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}], readonly ["nctraj", {
    label: string;
    description: string;
    category: string;
    binaryExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}], readonly ["lammpstrj", {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: import("../../mol-plugin/context.js").PluginContext, data: import("../../mol-state/index.js").StateObjectRef<import("../objects.js").PluginStateObject.Data.String | import("../objects.js").PluginStateObject.Data.Binary>) => Promise<import("../../mol-state/index.js").StateObjectSelector<import("../objects.js").PluginStateObject.Molecule.Coordinates, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}]];
export type BuiltInCoordinatesFormat = (typeof BuiltInCoordinatesFormats)[number][0];
