/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { PluginContext } from '../../mol-plugin/context.js';
import { StateObjectRef } from '../../mol-state/index.js';
import { PluginStateObject } from '../objects.js';
export declare const ShapeFormatCategory = "Shape";
export declare const PlyProvider: {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: PluginContext, data: StateObjectRef<PluginStateObject.Data.String | PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Format.Ply, import("../../mol-state/index.js").StateTransformer<PluginStateObject.Data.String, PluginStateObject.Format.Ply, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        shape: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Provider, import("../../mol-state/index.js").StateTransformer<PluginStateObject.Format.Ply, PluginStateObject.Shape.Provider, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
            transforms: import("../../mol-math/linear-algebra.js").Mat4[] | undefined;
            label: string | undefined;
        }>>>;
    }>;
    visuals(plugin: PluginContext, data: {
        shape: StateObjectRef<PluginStateObject.Shape.Provider>;
    }): Promise<import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
};
export declare const BuiltInShapeFormats: readonly [readonly ["ply", {
    label: string;
    description: string;
    category: string;
    stringExtensions: string[];
    parse: (plugin: PluginContext, data: StateObjectRef<PluginStateObject.Data.String | PluginStateObject.Data.Binary>) => Promise<{
        format: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Format.Ply, import("../../mol-state/index.js").StateTransformer<PluginStateObject.Data.String, PluginStateObject.Format.Ply, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{}>>>;
        shape: import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Provider, import("../../mol-state/index.js").StateTransformer<PluginStateObject.Format.Ply, PluginStateObject.Shape.Provider, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
            transforms: import("../../mol-math/linear-algebra.js").Mat4[] | undefined;
            label: string | undefined;
        }>>>;
    }>;
    visuals(plugin: PluginContext, data: {
        shape: StateObjectRef<PluginStateObject.Shape.Provider>;
    }): Promise<import("../../mol-state/index.js").StateObjectSelector<PluginStateObject.Shape.Representation3D, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}]];
export type BuildInShapeFormat = (typeof BuiltInShapeFormats)[number][0];
