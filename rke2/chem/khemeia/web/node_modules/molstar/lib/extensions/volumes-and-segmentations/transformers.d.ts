/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { StateTransformer } from '../../mol-state/index.js';
import { VolsegEntry } from './entry-root.js';
import { VolsegState } from './entry-state.js';
import { VolsegGlobalState } from './global-state.js';
export declare const VolsegEntryFromRoot: StateTransformer<PluginStateObject.Root, VolsegEntry, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
    serverUrl: string;
    source: "emdb" | "empiar" | "idr";
    entryId: string;
}>>;
export declare const VolsegStateFromEntry: StateTransformer<VolsegEntry, VolsegState, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
    volumeType: "off" | "direct-volume" | "isosurface";
    volumeIsovalueKind: string;
    volumeIsovalueValue: number;
    volumeOpacity: number;
    segmentOpacity: number;
    selectedSegment: number;
    visibleSegments: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
        segmentId: /*elided*/ any;
    }>[];
    visibleModels: import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
        pdbId: /*elided*/ any;
    }>[];
}>>;
export declare const VolsegGlobalStateFromRoot: StateTransformer<PluginStateObject.Root, VolsegGlobalState, import("../../mol-util/param-definition.js").ParamDefinition.Normalize<{
    tryUseGpu: boolean;
    selectionMode: boolean;
}>>;
