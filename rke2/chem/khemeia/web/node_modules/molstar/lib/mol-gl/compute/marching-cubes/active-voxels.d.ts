/**
 * Copyright (c) 2019-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../webgl/context.js';
import { Texture } from '../../../mol-gl/webgl/texture.js';
import { Vec3, Vec2 } from '../../../mol-math/linear-algebra.js';
export declare function calcActiveVoxels(ctx: WebGLContext, volumeData: Texture, gridDim: Vec3, gridTexDim: Vec3, isoValue: number, gridScale: Vec2): Texture;
