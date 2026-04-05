"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSTrajectoryWithCoordinates = void 0;
const objects_1 = require("../../../mol-plugin-state/objects.js");
const model_1 = require("../../../mol-plugin-state/transforms/model.js");
const mol_task_1 = require("../../../mol-task/index.js");
const param_definition_1 = require("../../../mol-util/param-definition.js");
const utils_1 = require("../helpers/utils.js");
const annotation_structure_component_1 = require("./annotation-structure-component.js");
exports.MVSTrajectoryWithCoordinates = (0, annotation_structure_component_1.MVSTransform)({
    name: 'trajectory-with-coordinates',
    display: { name: 'Trajectory with Coordinates', description: 'Create a trajectory from existing model and the provided coordinates.' },
    from: [objects_1.PluginStateObject.Molecule.Model, objects_1.PluginStateObject.Molecule.Topology],
    to: objects_1.PluginStateObject.Molecule.Trajectory,
    params: {
        coordinatesRef: param_definition_1.ParamDefinition.Text('', { isHidden: true }),
    }
})({
    apply({ a, params, dependencies }) {
        return mol_task_1.Task.create('Create trajectory from model/topology and coordinates', async (ctx) => {
            const coordinates = (0, utils_1.getMVSReferenceObject)([objects_1.PluginStateObject.Molecule.Coordinates], dependencies, params.coordinatesRef);
            if (!coordinates) {
                throw new Error('Coordinates not found.');
            }
            const trajectory = await (0, model_1.getTrajectory)(ctx, a, coordinates.data);
            const props = { label: 'Trajectory', description: `${trajectory.frameCount} model${trajectory.frameCount === 1 ? '' : 's'}` };
            return new objects_1.PluginStateObject.Molecule.Trajectory(trajectory, props);
        });
    }
});
