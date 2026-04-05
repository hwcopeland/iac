/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Unit, Structure } from '../../../../mol-model/structure.js';
import { Task } from '../../../../mol-task/index.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
import { Texture } from '../../../../mol-gl/webgl/texture.js';
import { WebGLContext } from '../../../../mol-gl/webgl/context.js';
import { SizeTheme } from '../../../../mol-theme/size.js';
export declare const GaussianDensityParams: {
    ignoreHydrogens: PD.BooleanParam;
    ignoreHydrogensVariant: PD.Select<"all" | "non-polar">;
    traceOnly: PD.BooleanParam;
    includeParent: PD.BooleanParam;
    resolution: PD.Numeric;
    radiusOffset: PD.Numeric;
    smoothness: PD.Numeric;
    floodfill: PD.Select<"inside" | "outside" | "off">;
};
export declare const DefaultGaussianDensityProps: PD.Values<{
    ignoreHydrogens: PD.BooleanParam;
    ignoreHydrogensVariant: PD.Select<"all" | "non-polar">;
    traceOnly: PD.BooleanParam;
    includeParent: PD.BooleanParam;
    resolution: PD.Numeric;
    radiusOffset: PD.Numeric;
    smoothness: PD.Numeric;
    floodfill: PD.Select<"inside" | "outside" | "off">;
}>;
export type GaussianDensityProps = typeof DefaultGaussianDensityProps;
export declare function getTextureMaxCells(webgl: WebGLContext, structure?: Structure): number;
export declare function computeUnitGaussianDensity(structure: Structure, unit: Unit, sizeTheme: SizeTheme<any>, props: GaussianDensityProps): Task<import("../../../../mol-math/geometry/gaussian-density.js").GaussianDensityData>;
export declare function computeUnitGaussianDensityTexture(structure: Structure, unit: Unit, sizeTheme: SizeTheme<any>, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): import("../../../../mol-math/geometry/gaussian-density.js").GaussianDensityTextureData;
export declare function computeUnitGaussianDensityTexture2d(structure: Structure, unit: Unit, sizeTheme: SizeTheme<any>, powerOfTwo: boolean, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): import("../../../../mol-math/geometry/gaussian-density.js").GaussianDensityTextureData;
export declare function computeStructureGaussianDensity(structure: Structure, sizeTheme: SizeTheme<any>, props: GaussianDensityProps): Task<import("../../../../mol-math/geometry/gaussian-density.js").GaussianDensityData>;
export declare function computeStructureGaussianDensityTexture(structure: Structure, sizeTheme: SizeTheme<any>, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): import("../../../../mol-math/geometry/gaussian-density.js").GaussianDensityTextureData;
export declare function computeStructureGaussianDensityTexture2d(structure: Structure, sizeTheme: SizeTheme<any>, powerOfTwo: boolean, props: GaussianDensityProps, webgl: WebGLContext, texture?: Texture): import("../../../../mol-math/geometry/gaussian-density.js").GaussianDensityTextureData;
