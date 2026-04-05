/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Program } from './webgl/program.js';
import { RenderableValues, Values, RenderableSchema, BaseValues } from './renderable/schema.js';
import { GraphicsRenderItem, ComputeRenderItem, GraphicsRenderVariant, Transparency } from './webgl/render-item.js';
import { Frustum3D } from '../mol-math/geometry/primitives/frustum3d.js';
import { Plane3D } from '../mol-math/geometry/primitives/plane3d.js';
import { Sphere3D } from '../mol-math/geometry/primitives/sphere3d.js';
import { WebGLStats } from './webgl/context.js';
export type RenderableState = {
    disposed: boolean;
    visible: boolean;
    alphaFactor: number;
    pickable: boolean;
    colorOnly: boolean;
    opaque: boolean;
    writeDepth: boolean;
};
export interface Renderable<T extends RenderableValues> {
    readonly id: number;
    readonly materialId: number;
    readonly values: T;
    readonly state: RenderableState;
    cull: (cameraPlane: Plane3D, frustum: Frustum3D, isOccluded: ((s: Sphere3D) => boolean) | null, stats: WebGLStats) => void;
    uncull: () => void;
    cullSimple: (d: number, radius: number, scale: number) => void;
    render: (variant: GraphicsRenderVariant, sharedTexturesCount: number) => void;
    getByteCount: () => number;
    getProgram: (variant: GraphicsRenderVariant) => Program;
    setTransparency: (transparency: Transparency) => void;
    update: () => void;
    dispose: () => void;
}
type GraphicsRenderableValues = RenderableValues & BaseValues;
export declare function createRenderable<T extends GraphicsRenderableValues>(renderItem: GraphicsRenderItem, values: T, state: RenderableState): Renderable<T>;
export type GraphicsRenderable = Renderable<GraphicsRenderableValues>;
export interface ComputeRenderable<T extends RenderableValues> {
    readonly id: number;
    readonly values: T;
    render: () => void;
    update: () => void;
    dispose: () => void;
}
export declare function createComputeRenderable<T extends Values<RenderableSchema>>(renderItem: ComputeRenderItem, values: T): ComputeRenderable<T>;
export {};
