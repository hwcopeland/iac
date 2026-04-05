/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { PluginStateObject } from '../../../mol-plugin-state/objects.js';
import { Choice } from '../../../mol-util/param-choice.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
export declare const DEFAULT_MESH_SERVER = "http://localhost:9000/v2";
declare const MeshServerInfo_base: {
    new (data: PD.Values<{
        serverUrl: PD.Text<string>;
        source: PD.Select<"emdb" | "empiar">;
        entryId: PD.Text<string>;
    }>, props?: {
        label: string;
        description?: string;
    } | undefined): {
        id: import("../../../mol-util/index.js").UUID;
        type: PluginStateObject.TypeInfo;
        label: string;
        description?: string;
        data: PD.Values<{
            serverUrl: PD.Text<string>;
            source: PD.Select<"emdb" | "empiar">;
            entryId: PD.Text<string>;
        }>;
    };
    type: PluginStateObject.TypeInfo;
    is(obj?: import("../../../mol-state/index.js").StateObject): obj is import("../../../mol-state/index.js").StateObject<PD.Values<{
        serverUrl: PD.Text<string>;
        source: PD.Select<"emdb" | "empiar">;
        entryId: PD.Text<string>;
    }>, PluginStateObject.TypeInfo>;
};
export declare class MeshServerInfo extends MeshServerInfo_base {
}
export declare namespace MeshServerInfo {
    const MeshSourceChoice: Choice<"emdb" | "empiar", "empiar">;
    type MeshSource = Choice.Values<typeof MeshSourceChoice>;
    const Params: {
        serverUrl: PD.Text<string>;
        source: PD.Select<"emdb" | "empiar">;
        entryId: PD.Text<string>;
    };
    type Data = PD.Values<typeof Params>;
}
export {};
