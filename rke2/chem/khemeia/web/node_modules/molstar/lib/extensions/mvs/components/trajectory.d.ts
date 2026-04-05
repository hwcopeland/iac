/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { PluginStateObject } from '../../../mol-plugin-state/objects.js';
import { ParamDefinition } from '../../../mol-util/param-definition.js';
export declare const MVSTrajectoryWithCoordinates: import("../../../mol-state/index.js").StateTransformer<PluginStateObject.Molecule.Topology | PluginStateObject.Molecule.Model, PluginStateObject.Molecule.Trajectory, ParamDefinition.Normalize<{
    coordinatesRef: string;
}>>;
