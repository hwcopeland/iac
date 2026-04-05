/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Texture } from '../../mol-gl/webgl/texture.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Viewport } from '../camera/util.js';
import { RenderTarget } from '../../mol-gl/webgl/render-target.js';
export declare const CasParams: {
    sharpness: PD.Numeric;
    denoise: PD.BooleanParam;
};
export type CasProps = PD.Values<typeof CasParams>;
export declare class CasPass {
    private webgl;
    private readonly renderable;
    constructor(webgl: WebGLContext, input: Texture);
    private updateState;
    setSize(width: number, height: number): void;
    update(input: Texture, props: CasProps): void;
    render(viewport: Viewport, target: RenderTarget | undefined): void;
}
