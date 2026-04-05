/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Box3D, DensityData } from '../geometry.js';
import { PositionData } from './common.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Texture } from '../../mol-gl/webgl/texture.js';
import { Task } from '../../mol-task/task.js';
import { Mat4 } from '../linear-algebra/3d/mat4.js';
import { Vec3 } from '../linear-algebra/3d/vec3.js';
import { Vec2 } from '../linear-algebra/3d/vec2.js';
export declare const DefaultGaussianDensityProps: {
    resolution: number;
    radiusOffset: number;
    smoothness: number;
};
export type GaussianDensityProps = typeof DefaultGaussianDensityProps;
export type GaussianDensityData = {
    radiusFactor: number;
} & DensityData;
export type GaussianDensityTextureData = {
    radiusFactor: number;
    resolution: number;
    maxRadius: number;
    transform: Mat4;
    texture: Texture;
    bbox: Box3D;
    gridDim: Vec3;
    gridTexDim: Vec3;
    gridDataDim: Vec3;
    gridTexScale: Vec2;
};
export declare function computeGaussianDensity(position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps): Task<GaussianDensityData>;
export declare function computeGaussianDensityTexture(position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): Task<GaussianDensityTextureData>;
export declare function computeGaussianDensityTexture2d(position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): Task<GaussianDensityTextureData>;
export declare function computeGaussianDensityTexture3d(position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): Task<GaussianDensityTextureData>;
