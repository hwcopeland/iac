/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { OptionalField, RequiredField, ValueFor } from '../generic/field-schema.js';
import { SimpleParamsSchema, UnionParamsSchema } from '../generic/params-schema.js';
import { NodeFor, ParamsOfKind, SubtreeOfKind, TreeFor, TreeSchema } from '../generic/tree-schema.js';
import { ColorT, ContinuousPalette, DiscretePalette, Vector3 } from '../mvs/param-types.js';
type Easing = 'linear' | 'bounce-in' | 'bounce-out' | 'bounce-in-out' | 'circle-in' | 'circle-out' | 'circle-in-out' | 'cubic-in' | 'cubic-out' | 'cubic-in-out' | 'exp-in' | 'exp-out' | 'exp-in-out' | 'quad-in' | 'quad-out' | 'quad-in-out' | 'sin-in' | 'sin-out' | 'sin-in-out';
declare const Easing: import("io-ts").Type<Easing, Easing, unknown>;
export type MVSAnimationEasing = ValueFor<typeof Easing>;
export declare const MVSAnimationSchema: TreeSchema<{
    animation: SimpleParamsSchema<{
        /** Frame time in milliseconds. */
        frame_time_ms: OptionalField<number>;
        /** Total duration of the animation. If not specified, computed as maximum of all transitions. */
        duration_ms: OptionalField<number | null>;
        /** Determines whether the animation should autoplay when a snapshot is loaded */
        autoplay: OptionalField<boolean>;
        /** Determines whether the animation should loop when it reaches the end. */
        loop: OptionalField<boolean>;
        /** Determines whether the camera state should be included in the animation. */
        include_camera: OptionalField<boolean>;
        /** Determines whether the canvas state should be included in the animation. */
        include_canvas: OptionalField<boolean>;
    }>;
    interpolate: UnionParamsSchema<"kind", {
        scalar: SimpleParamsSchema<{
            /** Magnitude of the noise to apply to the interpolated value. */
            noise_magnitude: OptionalField<number>;
            /** Start value. If a list of values is provided, each element will be interpolated separately. If unset, parent state value is used. */
            start: OptionalField<number | number[] | null>;
            /** End value. If a list of values is provided, each element will be interpolated separately. If unset, only noise is applied. */
            end: OptionalField<number | number[] | null>;
            /** Whether to round the values to the closest integer. Useful for example for trajectory animation. */
            discrete: OptionalField<boolean>;
            /** Easing function to use for the transition. */
            easing: OptionalField<Easing>;
            /** Determines how many times the interpolation loops. Current T = frequency * t mod 1. */
            frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            alternate_direction: OptionalField<boolean>;
            /** Reference to the node. */
            target_ref: RequiredField<string>;
            /** Value accessor. */
            property: RequiredField<string | (string | number)[]>;
            /** Start time of the transition in milliseconds. */
            start_ms: OptionalField<number>;
            /** Duration of the transition in milliseconds. */
            duration_ms: RequiredField<number>;
        }>;
        vec3: SimpleParamsSchema<{
            /** Magnitude of the noise to apply to the interpolated value. */
            noise_magnitude: OptionalField<number>;
            /** Start value. If unset, parent state value is used. Must be array of length 3N (x1, y1, z1, x2, y2, z2, ...). */
            start: OptionalField<number[] | null>;
            /** End value. Must be array of length 3N (x1, y1, z1, x2, y2, z2, ...). If unset, only noise is applied. */
            end: OptionalField<number[] | null>;
            /** Whether to use spherical interpolation. */
            spherical: OptionalField<boolean>;
            /** Easing function to use for the transition. */
            easing: OptionalField<Easing>;
            /** Determines how many times the interpolation loops. Current T = frequency * t mod 1. */
            frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            alternate_direction: OptionalField<boolean>;
            /** Reference to the node. */
            target_ref: RequiredField<string>;
            /** Value accessor. */
            property: RequiredField<string | (string | number)[]>;
            /** Start time of the transition in milliseconds. */
            start_ms: OptionalField<number>;
            /** Duration of the transition in milliseconds. */
            duration_ms: RequiredField<number>;
        }>;
        rotation_matrix: SimpleParamsSchema<{
            /** Magnitude of the noise to apply to the interpolated value. */
            noise_magnitude: OptionalField<number>;
            /** Start value. If unset, parent state value is used. */
            start: OptionalField<number[] | null>;
            /** End value. If unset, only noise is applied. */
            end: OptionalField<number[] | null>;
            /** Easing function to use for the transition. */
            easing: OptionalField<Easing>;
            /** Determines how many times the interpolation loops. Current T = frequency * t mod 1. */
            frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            alternate_direction: OptionalField<boolean>;
            /** Reference to the node. */
            target_ref: RequiredField<string>;
            /** Value accessor. */
            property: RequiredField<string | (string | number)[]>;
            /** Start time of the transition in milliseconds. */
            start_ms: OptionalField<number>;
            /** Duration of the transition in milliseconds. */
            duration_ms: RequiredField<number>;
        }>;
        transform_matrix: SimpleParamsSchema<{
            /** Pivot point for rotation and scale. */
            pivot: OptionalField<Vector3 | null>;
            /** Start rotation value. If unset, parent state value is used. */
            rotation_start: OptionalField<number[] | null>;
            /** End rotation value. If unset, only noise is applied */
            rotation_end: OptionalField<number[] | null>;
            /** Magnitude of the noise to apply to the rotation. */
            rotation_noise_magnitude: OptionalField<number>;
            /** Easing function to use for the rotation. */
            rotation_easing: OptionalField<Easing>;
            /** Determines how many times the rotation interpolation loops. Current T = frequency * t mod 1. */
            rotation_frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            rotation_alternate_direction: OptionalField<boolean>;
            /** Start translation value. If unset, parent state value is used. */
            translation_start: OptionalField<Vector3 | null>;
            /** End translation value. If unset, only noise is applied. */
            translation_end: OptionalField<Vector3 | null>;
            /** Magnitude of the noise to apply to the translation. */
            translation_noise_magnitude: OptionalField<number>;
            /** Easing function to use for the translation. */
            translation_easing: OptionalField<Easing>;
            /** Determines how many times the translation interpolation loops. Current T = frequency * t mod 1. */
            translation_frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            translation_alternate_direction: OptionalField<boolean>;
            /** Start scale value. If unset, parent state value is used. */
            scale_start: OptionalField<Vector3 | null>;
            /** End scale value. If unset, only noise is applied. */
            scale_end: OptionalField<Vector3 | null>;
            /** Magnitude of the noise to apply to the scale. */
            scale_noise_magnitude: OptionalField<number>;
            /** Easing function to use for the scale. */
            scale_easing: OptionalField<Easing>;
            /** Determines how many times the scale interpolation loops. Current T = frequency * t mod 1. */
            scale_frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            scale_alternate_direction: OptionalField<boolean>;
            /** Reference to the node. */
            target_ref: RequiredField<string>;
            /** Value accessor. */
            property: RequiredField<string | (string | number)[]>;
            /** Start time of the transition in milliseconds. */
            start_ms: OptionalField<number>;
            /** Duration of the transition in milliseconds. */
            duration_ms: RequiredField<number>;
        }>;
        color: SimpleParamsSchema<{
            /** Start value. If unset, parent state value is used. */
            start: OptionalField<ColorT | {
                [x: string]: ColorT;
                [x: number]: ColorT;
            } | null>;
            /** End value. */
            end: OptionalField<ColorT | {
                [x: string]: ColorT;
                [x: number]: ColorT;
            } | null>;
            /** Palette to sample colors from. Overrides start and end values. */
            palette: OptionalField<DiscretePalette | ContinuousPalette | null>;
            /** Easing function to use for the transition. */
            easing: OptionalField<Easing>;
            /** Determines how many times the interpolation loops. Current T = frequency * t mod 1. */
            frequency: OptionalField<number>;
            /** Whether to alternate the direction of the interpolation for frequency > 1. */
            alternate_direction: OptionalField<boolean>;
            /** Reference to the node. */
            target_ref: RequiredField<string>;
            /** Value accessor. */
            property: RequiredField<string | (string | number)[]>;
            /** Start time of the transition in milliseconds. */
            start_ms: OptionalField<number>;
            /** Duration of the transition in milliseconds. */
            duration_ms: RequiredField<number>;
        }>;
    }>;
}, "animation">;
export type MVSAnimationKind = keyof typeof MVSAnimationSchema.nodes;
export type MVSAnimationNode<TKind extends MVSAnimationKind = MVSAnimationKind> = NodeFor<typeof MVSAnimationSchema, TKind>;
export type MVSAnimationTree = TreeFor<typeof MVSAnimationSchema>;
export type MVSAnimationNodeParams<TKind extends MVSAnimationKind> = ParamsOfKind<MVSAnimationTree, TKind>;
export type MVSAnimationSubtree<TKind extends MVSAnimationKind = MVSAnimationKind> = SubtreeOfKind<MVSAnimationTree, TKind>;
export {};
