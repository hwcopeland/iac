"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSAnimationSchema = void 0;
const field_schema_1 = require("../generic/field-schema.js");
const params_schema_1 = require("../generic/params-schema.js");
const tree_schema_1 = require("../generic/tree-schema.js");
const param_types_1 = require("../mvs/param-types.js");
const Easing = (0, field_schema_1.literal)('linear', 'bounce-in', 'bounce-out', 'bounce-in-out', 'circle-in', 'circle-out', 'circle-in-out', 'cubic-in', 'cubic-out', 'cubic-in-out', 'exp-in', 'exp-out', 'exp-in-out', 'quad-in', 'quad-out', 'quad-in-out', 'sin-in', 'sin-out', 'sin-in-out');
const _Noise = {
    /** Magnitude of the noise to apply to the interpolated value. */
    noise_magnitude: (0, field_schema_1.OptionalField)(field_schema_1.float, 0, 'Magnitude of the noise to apply to the interpolated value.')
    // support cummulative noise?
};
const _Common = {
    /** Reference to the node. */
    target_ref: (0, field_schema_1.RequiredField)(field_schema_1.str, 'Reference to the node.'),
    /** Value accessor. */
    property: (0, field_schema_1.RequiredField)((0, field_schema_1.union)(field_schema_1.str, (0, field_schema_1.list)((0, field_schema_1.union)(field_schema_1.str, field_schema_1.int))), 'Value accessor.'),
    /** Start time of the transition in milliseconds. */
    start_ms: (0, field_schema_1.OptionalField)(field_schema_1.float, 0, 'Start time of the transition in milliseconds.'),
    /** Duration of the transition in milliseconds. */
    duration_ms: (0, field_schema_1.RequiredField)(field_schema_1.float, 'Duration of the transition in milliseconds.'),
};
const _Frequency = {
    /** Determines how many times the interpolation loops. Current T = frequency * t mod 1. */
    frequency: (0, field_schema_1.OptionalField)(field_schema_1.int, 1, 'Determines how many times the interpolation loops. Current T = frequency * t mod 1.'),
    /** Whether to alternate the direction of the interpolation for frequency > 1. */
    alternate_direction: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Whether to alternate the direction of the interpolation for frequency > 1.'),
};
const _Easing = {
    /** Easing function to use for the transition. */
    easing: (0, field_schema_1.OptionalField)(Easing, 'linear', 'Easing function to use for the transition.'),
};
const ScalarInterpolation = {
    ..._Common,
    ..._Frequency,
    ..._Easing,
    /** Start value. If a list of values is provided, each element will be interpolated separately. If unset, parent state value is used. */
    start: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)((0, field_schema_1.union)(field_schema_1.float, (0, field_schema_1.list)(field_schema_1.float))), null, 'Start value. If a list of values is provided, each element will be interpolated separately. If unset, parent state value is used.'),
    /** End value. If a list of values is provided, each element will be interpolated separately. If unset, only noise is applied. */
    end: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)((0, field_schema_1.union)(field_schema_1.float, (0, field_schema_1.list)(field_schema_1.float))), null, 'End value. If a list of values is provided, each element will be interpolated separately. If unset, only noise is applied.'),
    /** Whether to round the values to the closest integer. Useful for example for trajectory animation. */
    discrete: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Whether to round the values to the closest integer. Useful for example for trajectory animation.'),
    ..._Noise,
};
const Vec3Interpolation = {
    ..._Common,
    ..._Frequency,
    ..._Easing,
    /** Start value. If unset, parent state value is used. Must be array of length 3N (x1, y1, z1, x2, y2, z2, ...). */
    start: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)((0, field_schema_1.list)(field_schema_1.float)), null, 'Start value. If unset, parent state value is used. Must be array of length 3N (x1, y1, z1, x2, y2, z2, ...).'),
    /** End value. Must be array of length 3N (x1, y1, z1, x2, y2, z2, ...). If unset, only noise is applied. */
    end: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)((0, field_schema_1.list)(field_schema_1.float)), null, 'End value. Must be array of length 3N (x1, y1, z1, x2, y2, z2, ...). If unset, only noise is applied.'),
    /** Whether to use spherical interpolation. */
    spherical: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Whether to use spherical interpolation.'),
    ..._Noise,
};
const RotationMatrixInterpolation = {
    ..._Common,
    ..._Frequency,
    ..._Easing,
    /** Start value. If unset, parent state value is used. */
    start: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Matrix), null, 'Start value. If unset, parent state value is used.'),
    /** End value. If unset, only noise is applied. */
    end: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Matrix), null, 'End value. If unset, only noise is applied.'),
    ..._Noise,
};
const ColorInterpolation = {
    ..._Common,
    ..._Frequency,
    ..._Easing,
    /** Start value. If unset, parent state value is used. */
    start: (0, field_schema_1.OptionalField)((0, field_schema_1.union)((0, field_schema_1.nullable)(param_types_1.ColorT), (0, field_schema_1.dict)((0, field_schema_1.union)(field_schema_1.int, field_schema_1.str), param_types_1.ColorT)), null, 'Start value. If unset, parent state value is used.'),
    /** End value. */
    end: (0, field_schema_1.OptionalField)((0, field_schema_1.union)((0, field_schema_1.nullable)(param_types_1.ColorT), (0, field_schema_1.dict)((0, field_schema_1.union)(field_schema_1.int, field_schema_1.str), param_types_1.ColorT)), null, 'End value.'),
    /** Palette to sample colors from. Overrides start and end values. */
    palette: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)((0, field_schema_1.union)(param_types_1.DiscretePalette, param_types_1.ContinuousPalette)), null, 'Palette to sample colors from. Overrides start and end values.'),
};
const TransformationMatrixInterpolation = {
    ..._Common,
    /** Pivot point for rotation and scale. */
    pivot: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Vector3), null, 'Pivot point for rotation and scale.'),
    /** Start rotation value. If unset, parent state value is used. */
    rotation_start: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Matrix), null, 'Start rotation value. If unset, parent state value is used.'),
    /** End rotation value. If unset, only noise is applied */
    rotation_end: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Matrix), null, 'End rotation value. If unset, only noise is applied.'),
    /** Magnitude of the noise to apply to the rotation. */
    rotation_noise_magnitude: (0, field_schema_1.OptionalField)(field_schema_1.float, 0, 'Magnitude of the noise to apply to the rotation.'),
    /** Easing function to use for the rotation. */
    rotation_easing: (0, field_schema_1.OptionalField)(Easing, 'linear', 'Easing function to use for the rotation.'),
    /** Determines how many times the rotation interpolation loops. Current T = frequency * t mod 1. */
    rotation_frequency: (0, field_schema_1.OptionalField)(field_schema_1.int, 1, 'Determines how many times the rotation interpolation loops. Current T = frequency * t mod 1.'),
    /** Whether to alternate the direction of the interpolation for frequency > 1. */
    rotation_alternate_direction: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Whether to alternate the direction of the interpolation for frequency > 1.'),
    /** Start translation value. If unset, parent state value is used. */
    translation_start: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Vector3), null, 'Start translation value. If unset, parent state value is used.'),
    /** End translation value. If unset, only noise is applied. */
    translation_end: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Vector3), null, 'End translation value. If unset, only noise is applied.'),
    /** Magnitude of the noise to apply to the translation. */
    translation_noise_magnitude: (0, field_schema_1.OptionalField)(field_schema_1.float, 0, 'Magnitude of the noise to apply to the translation.'),
    /** Easing function to use for the translation. */
    translation_easing: (0, field_schema_1.OptionalField)(Easing, 'linear', 'Easing function to use for the translation.'),
    /** Determines how many times the translation interpolation loops. Current T = frequency * t mod 1. */
    translation_frequency: (0, field_schema_1.OptionalField)(field_schema_1.int, 1, 'Determines how many times the translation interpolation loops. Current T = frequency * t mod 1.'),
    /** Whether to alternate the direction of the interpolation for frequency > 1. */
    translation_alternate_direction: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Whether to alternate the direction of the interpolation for frequency > 1.'),
    /** Start scale value. If unset, parent state value is used. */
    scale_start: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Vector3), null, 'Start scale value. If unset, parent state value is used.'),
    /** End scale value. If unset, only noise is applied. */
    scale_end: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(param_types_1.Vector3), null, 'End scale value. If unset, only noise is applied.'),
    /** Magnitude of the noise to apply to the scale. */
    scale_noise_magnitude: (0, field_schema_1.OptionalField)(field_schema_1.float, 0, 'Magnitude of the noise to apply to the scale.'),
    /** Easing function to use for the scale. */
    scale_easing: (0, field_schema_1.OptionalField)(Easing, 'linear', 'Easing function to use for the scale.'),
    /** Determines how many times the scale interpolation loops. Current T = frequency * t mod 1. */
    scale_frequency: (0, field_schema_1.OptionalField)(field_schema_1.int, 1, 'Determines how many times the scale interpolation loops. Current T = frequency * t mod 1.'),
    /** Whether to alternate the direction of the interpolation for frequency > 1. */
    scale_alternate_direction: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Whether to alternate the direction of the interpolation for frequency > 1.'),
};
exports.MVSAnimationSchema = (0, tree_schema_1.TreeSchema)({
    rootKind: 'animation',
    nodes: {
        animation: {
            description: 'Animation root node',
            parent: [],
            params: (0, params_schema_1.SimpleParamsSchema)({
                /** Frame time in milliseconds. */
                frame_time_ms: (0, field_schema_1.OptionalField)(field_schema_1.float, 1000 / 60, 'Frame time in milliseconds.'),
                /** Total duration of the animation. If not specified, computed as maximum of all transitions. */
                duration_ms: (0, field_schema_1.OptionalField)((0, field_schema_1.nullable)(field_schema_1.float), null, 'Total duration of the animation. If not specified, computed as maximum of all transitions.'),
                /** Determines whether the animation should autoplay when a snapshot is loaded */
                autoplay: (0, field_schema_1.OptionalField)(field_schema_1.bool, true, 'Determines whether the animation should autoplay when a snapshot is loaded.'),
                /** Determines whether the animation should loop when it reaches the end. */
                loop: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Determines whether the animation should loop when it reaches the end.'),
                /** Determines whether the camera state should be included in the animation. */
                include_camera: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Determines whether the camera state should be included in the animation.'),
                /** Determines whether the canvas state should be included in the animation. */
                include_canvas: (0, field_schema_1.OptionalField)(field_schema_1.bool, false, 'Determines whether the canvas state should be included in the animation.'),
            }),
        },
        interpolate: {
            description: 'This node enables interpolating between values',
            parent: ['animation'],
            params: (0, params_schema_1.UnionParamsSchema)('kind', 'Interpolation kind', {
                scalar: (0, params_schema_1.SimpleParamsSchema)(ScalarInterpolation),
                vec3: (0, params_schema_1.SimpleParamsSchema)(Vec3Interpolation),
                rotation_matrix: (0, params_schema_1.SimpleParamsSchema)(RotationMatrixInterpolation),
                transform_matrix: (0, params_schema_1.SimpleParamsSchema)(TransformationMatrixInterpolation),
                color: (0, params_schema_1.SimpleParamsSchema)(ColorInterpolation),
            })
        }
    }
});
