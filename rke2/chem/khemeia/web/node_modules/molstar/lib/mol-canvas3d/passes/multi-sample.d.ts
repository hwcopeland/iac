/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { RenderTarget } from '../../mol-gl/webgl/render-target.js';
import { Camera } from '../../mol-canvas3d/camera.js';
import { PostprocessingProps } from './postprocessing.js';
import { DrawPass } from './draw.js';
import { Renderer } from '../../mol-gl/renderer.js';
import { Scene } from '../../mol-gl/scene.js';
import { Helper } from '../helper/helper.js';
import { StereoCamera } from '../camera/stereo.js';
import { MarkingProps } from './marking.js';
export declare const MultiSampleParams: {
    mode: PD.Select<string>;
    sampleLevel: PD.Numeric;
    reduceFlicker: PD.BooleanParam;
    reuseOcclusion: PD.BooleanParam;
};
export type MultiSampleProps = PD.Values<typeof MultiSampleParams>;
type Props = {
    multiSample: MultiSampleProps;
    postprocessing: PostprocessingProps;
    marking: MarkingProps;
    transparentBackground: boolean;
    dpoitIterations: number;
};
type RenderContext = {
    renderer: Renderer;
    camera: Camera | StereoCamera;
    scene: Scene;
    helper: Helper;
};
export declare class MultiSamplePass {
    private webgl;
    private drawPass;
    static isEnabled(props: MultiSampleProps): boolean;
    colorTarget: RenderTarget;
    private composeTarget;
    private holdTarget;
    private compose;
    constructor(webgl: WebGLContext, drawPass: DrawPass);
    getByteCount(): number;
    syncSize(): void;
    render(sampleIndex: number, ctx: RenderContext, props: Props, toDrawingBuffer: boolean, forceOn: boolean): number;
    private bindOutputTarget;
    private renderMultiSample;
    private renderTemporalMultiSample;
}
export declare const JitterVectors: number[][][];
export declare class MultiSampleHelper {
    private multiSamplePass;
    private sampleIndex;
    update(changed: boolean, props: MultiSampleProps): boolean;
    /** Return `true` while more samples are needed */
    render(ctx: RenderContext, props: Props, toDrawingBuffer: boolean, forceOn?: boolean): boolean;
    constructor(multiSamplePass: MultiSamplePass);
}
export {};
