/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Áron Samuel Kovács <aron.kovacs@mail.muni.cz>
 * @author Ludovic Autin <ludovic.autin@gmail.com>
 * @author Gianluca Tomasello <giagitom@gmail.com>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Texture } from '../../mol-gl/webgl/texture.js';
import { Vec3 } from '../../mol-math/linear-algebra.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { RenderTarget } from '../../mol-gl/webgl/render-target.js';
import { ICamera } from '../../mol-canvas3d/camera.js';
import { Light } from '../../mol-gl/renderer.js';
import { PostprocessingProps } from './postprocessing.js';
export declare const ShadowParams: {
    steps: PD.Numeric;
    maxDistance: PD.Numeric;
    tolerance: PD.Numeric;
};
export type ShadowProps = PD.Values<typeof ShadowParams>;
export declare class ShadowPass {
    readonly webgl: WebGLContext;
    static isEnabled(props: PostprocessingProps): boolean;
    readonly target: RenderTarget;
    private readonly renderable;
    private invProjection;
    private invHeadRotation;
    constructor(webgl: WebGLContext, width: number, height: number, depthTextureOpaque: Texture);
    getByteCount(): number;
    setSize(width: number, height: number): void;
    update(camera: ICamera, light: Light, ambientColor: Vec3, props: ShadowProps): void;
    render(): void;
}
