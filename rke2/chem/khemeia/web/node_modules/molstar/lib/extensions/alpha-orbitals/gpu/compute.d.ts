/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { WebGLContext } from '../../../mol-gl/webgl/context.js';
import { RuntimeContext } from '../../../mol-task/index.js';
import { AlphaOrbital, CubeGridInfo } from '../data-model.js';
export declare function gpuComputeAlphaOrbitalsGridValues(ctx: RuntimeContext, webgl: WebGLContext, grid: CubeGridInfo, orbital: AlphaOrbital): Promise<Float32Array<ArrayBuffer>>;
export declare function gpuComputeAlphaOrbitalsDensityGridValues(ctx: RuntimeContext, webgl: WebGLContext, grid: CubeGridInfo, orbitals: AlphaOrbital[]): Promise<Float32Array<ArrayBuffer>>;
