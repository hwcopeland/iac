/**
 * Copyright (c) 2017-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Michael Krone <michael.krone@uni-tuebingen.de>
 */
import { PositionData } from '../common.js';
import { Box3D } from '../../geometry.js';
import { GaussianDensityProps, GaussianDensityData, GaussianDensityTextureData } from '../gaussian-density.js';
import { WebGLContext } from '../../../mol-gl/webgl/context.js';
import { Texture } from '../../../mol-gl/webgl/texture.js';
export declare function GaussianDensityGPU(position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps, webgl: WebGLContext): GaussianDensityData;
export declare function GaussianDensityTexture(webgl: WebGLContext, position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps, oldTexture?: Texture): GaussianDensityTextureData;
export declare function GaussianDensityTexture2d(webgl: WebGLContext, position: PositionData, box: Box3D, radius: (index: number) => number, powerOfTwo: boolean, props: GaussianDensityProps, oldTexture?: Texture): GaussianDensityTextureData;
export declare function GaussianDensityTexture3d(webgl: WebGLContext, position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps, oldTexture?: Texture): GaussianDensityTextureData;
