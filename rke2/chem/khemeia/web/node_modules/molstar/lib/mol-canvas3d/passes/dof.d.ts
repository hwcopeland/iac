/**
 * Copyright (c) 2024-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Ludovic Autin <autin@scripps.edu>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Texture } from '../../mol-gl/webgl/texture.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Viewport } from '../camera/util.js';
import { RenderTarget } from '../../mol-gl/webgl/render-target.js';
import { ICamera } from '../../mol-canvas3d/camera.js';
import { Sphere3D } from '../../mol-math/geometry.js';
import { PostprocessingProps } from './postprocessing.js';
export declare const DofParams: {
    blurSize: PD.Numeric;
    blurSpread: PD.Numeric;
    inFocus: PD.Numeric;
    PPM: PD.Numeric;
    center: PD.Select<"camera-target" | "scene-center">;
    mode: PD.Select<"sphere" | "plane">;
};
export type DofProps = PD.Values<typeof DofParams>;
export declare class DofPass {
    private webgl;
    static isEnabled(props: PostprocessingProps): boolean;
    readonly target: RenderTarget;
    private readonly renderable;
    constructor(webgl: WebGLContext, width: number, height: number);
    getByteCount(): number;
    private updateState;
    setSize(width: number, height: number): void;
    update(camera: ICamera, input: Texture, depthOpaque: Texture, depthTransparent: Texture, props: DofProps, sphere: Sphere3D): void;
    render(viewport: Viewport, target: undefined | RenderTarget): void;
}
