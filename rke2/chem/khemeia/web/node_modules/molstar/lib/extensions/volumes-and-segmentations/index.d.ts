/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { PluginStateObject as SO } from '../../mol-plugin-state/objects.js';
import { PluginConfigItem } from '../../mol-plugin/config.js';
import { StateAction } from '../../mol-state/index.js';
export declare const VolsegVolumeServerConfig: {
    DefaultServer: PluginConfigItem<string>;
};
export declare const Volseg: import("../../mol-state/index.js").StateTransformer<any, any, any>;
export declare const LoadVolseg: StateAction<SO.Root, void, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
    serverUrl: string;
    source: import("../../mol-util/param-definition.js").ParamDefinition.NamedParams<unknown, string>;
}>>;
