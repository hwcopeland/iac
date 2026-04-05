/**
 * Copyright (c) 2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { ComputeRenderable } from '../renderable.js';
import { TextureSpec, UniformSpec, Values } from '../renderable/schema.js';
import { Texture } from '../webgl/texture.js';
declare const HiZSchema: {
    tPreviousLevel: TextureSpec<"texture">;
    uInvSize: UniformSpec<"v2">;
    uOffset: UniformSpec<"v2">;
    drawCount: import("../renderable/schema.js").ValueSpec<"number">;
    instanceCount: import("../renderable/schema.js").ValueSpec<"number">;
    aPosition: import("../renderable/schema.js").AttributeSpec<"float32">;
    uQuadScale: UniformSpec<"v2">;
};
export type HiZRenderable = ComputeRenderable<Values<typeof HiZSchema>>;
export declare function createHiZRenderable(ctx: WebGLContext, previousLevel: Texture): HiZRenderable;
export {};
