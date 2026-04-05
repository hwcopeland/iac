/**
 * Copyright (c) 2023-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { omitObjectKeys, pickObjectKeys } from '../../../../mol-util/object.js';
import { RequiredField, bool, str } from '../generic/field-schema.js';
import { SimpleParamsSchema } from '../generic/params-schema.js';
import { TreeSchema } from '../generic/tree-schema.js';
import { FullMVSTreeSchema } from '../mvs/mvs-tree.js';
import { MolstarParseFormatT } from '../mvs/param-types.js';
/** Schema for `MolstarTree` (intermediate tree representation between `MVSTree` and a real Molstar state) */
export const MolstarTreeSchema = TreeSchema({
    rootKind: 'root',
    nodes: {
        ...FullMVSTreeSchema.nodes,
        download: {
            ...FullMVSTreeSchema.nodes.download,
            params: SimpleParamsSchema({
                ...FullMVSTreeSchema.nodes.download.params.fields,
                /** Specifies whether file is downloaded as bytes array or string */
                is_binary: RequiredField(bool, 'Specifies whether file is downloaded as bytes array or string'),
            }),
        },
        parse: {
            ...FullMVSTreeSchema.nodes.parse,
            params: SimpleParamsSchema({
                /** File format */
                format: RequiredField(MolstarParseFormatT, 'File format'),
            }),
        },
        /** Auxiliary node corresponding to Molstar's CoordinatesFrom*. */
        coordinates: {
            description: "Auxiliary node corresponding to Molstar's CoordinatesFrom*.",
            parent: ['parse'],
            params: SimpleParamsSchema({
                /** File format */
                format: RequiredField(MolstarParseFormatT, 'File format'),
            }),
        },
        /** Auxiliary node corresponding to Molstar's TrajectoryFrom*. */
        trajectory: {
            description: "Auxiliary node corresponding to Molstar's TrajectoryFrom*.",
            parent: ['parse'],
            params: SimpleParamsSchema({
                /** File format */
                format: RequiredField(MolstarParseFormatT, 'File format'),
                ...pickObjectKeys(FullMVSTreeSchema.nodes.structure.params.fields, ['block_header', 'block_index']),
            }),
        },
        /** Auxiliary node corresponding to Molstar's TrajectoryFrom*. */
        trajectory_with_coordinates: {
            description: 'Auxiliary node corresponding to assigning a separate coordinates to a trajectory.',
            parent: ['model'],
            params: SimpleParamsSchema({
                /** Coordinates reference */
                coordinates_ref: RequiredField(str, 'Coordinates reference'),
            }),
        },
        topology_with_coordinates: {
            description: 'Auxiliary node corresponding to assigning a separate coordinates to a topology.',
            parent: ['parse'],
            params: SimpleParamsSchema({
                format: RequiredField(MolstarParseFormatT, 'File format'),
                coordinates_ref: RequiredField(str, 'Coordinates reference'),
            }),
        },
        /** Auxiliary node corresponding to Molstar's ModelFromTrajectory. */
        model: {
            description: "Auxiliary node corresponding to Molstar's ModelFromTrajectory.",
            parent: ['trajectory', 'trajectory_with_coordinates', 'topology_with_coordinates'],
            params: SimpleParamsSchema(pickObjectKeys(FullMVSTreeSchema.nodes.structure.params.fields, ['model_index'])),
        },
        /** Auxiliary node corresponding to Molstar's StructureFromModel. */
        structure: {
            ...FullMVSTreeSchema.nodes.structure,
            parent: ['model'],
            params: SimpleParamsSchema(omitObjectKeys(FullMVSTreeSchema.nodes.structure.params.fields, ['block_header', 'block_index', 'model_index', 'coordinates_ref'])),
        },
    }
});
