/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { Volume } from '../../mol-model/volume.js';
import { PluginStateObject } from '../../mol-plugin-state/objects.js';
import { StateBuilder, StateObjectSelector, StateTransformer } from '../../mol-state/index.js';
import { ParamDefinition } from '../../mol-util/param-definition.js';
import { Source } from './entry-root.js';
/** Split entry ID (e.g. 'emd-1832') into source ('emdb') and number ('1832') */
export declare function splitEntryId(entryId: string): {
    source: "emdb" | "empiar" | "idr";
    entryNumber: string;
};
/** Create entry ID (e.g. 'emd-1832') for a combination of source ('emdb') and number ('1832') */
export declare function createEntryId(source: Source, entryNumber: string | number): string;
export declare function isDefined<T>(x: T | undefined): x is T;
export declare class NodeManager {
    private nodes;
    constructor();
    private static nodeExists;
    getNode(key: string): StateObjectSelector | undefined;
    getNodes(): StateObjectSelector[];
    deleteAllNodes(update: StateBuilder.Root): void;
    hideAllNodes(): void;
    showNode(key: string, factory: () => StateObjectSelector | Promise<StateObjectSelector>, forceVisible?: boolean): Promise<StateObjectSelector<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>;
}
export declare const CreateVolume: StateTransformer<PluginStateObject.Root, PluginStateObject.Volume.Data, ParamDefinition.Normalize<{
    label: string;
    description: string;
    volume: Volume;
}>>;
export declare function applyEllipsis(name: string, max_chars?: number): string;
export declare function lazyGetter<T>(getter: () => T, errorIfUndefined?: string): () => T;
