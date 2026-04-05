/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Loci } from '../../mol-model/loci.js';
import { PluginContext } from '../../mol-plugin/context.js';
import { StateObjectSelector } from '../../mol-state/index.js';
import type { Snapshot } from './mvs-data.js';
import type { MVSNode } from './tree/mvs/mvs-tree.js';
/**
 * Queries all MolViewSpec references in the current state of the plugin.
 */
export declare function queryMVSRef(plugin: PluginContext, ref: string): import("../../mol-state/index.js").StateSelection.CellSeq<import("../../mol-state/index.js").StateObjectCell<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateTransform<import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>>>;
/**
 * Creates a mapping of all MolViewSpec references in the current state of the plugin.
 */
export declare function createMVSRefMap(plugin: PluginContext): Map<string, StateObjectSelector<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateTransformer<import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, import("../../mol-state/index.js").StateObject<any, import("../../mol-state/index.js").StateObject.Type<any>>, any>>[]>;
export declare function tryGetPrimitivesFromLoci(loci: Loci | undefined): MVSNode<'primitive'>[] | undefined;
export declare function getCurrentMVSSnapshot(plugin: PluginContext): Snapshot | undefined;
