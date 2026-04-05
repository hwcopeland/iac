/**
 * Copyright (c) 2022-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { StateObjectRef } from '../../../../mol-state/index.js';
import { PluginContext } from '../../../../mol-plugin/context.js';
import { PluginStateObject } from '../../../../mol-plugin-state/objects.js';
export declare function createPetworldHierarchy(plugin: PluginContext, trajectory: StateObjectRef<PluginStateObject.Molecule.Trajectory>): Promise<void>;
