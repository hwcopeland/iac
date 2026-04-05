/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Renderer } from '../../mol-gl/renderer.js';
import { Scene } from '../../mol-gl/scene.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Camera } from '../camera.js';
import { StereoCamera } from '../camera/stereo.js';
import { Viewport } from '../camera/util.js';
import { Helper } from '../helper/helper.js';
import { AsyncPickData, PickData, PickOptions, PickPass } from '../passes/pick.js';
export declare class PickHelper {
    private webgl;
    private renderer;
    private scene;
    private helper;
    private pickPass;
    dirty: boolean;
    private pickPadding;
    private buffers;
    private viewport;
    private pickRatio;
    private pickX;
    private pickY;
    private pickWidth;
    private pickHeight;
    private halfPickWidth;
    private spiral;
    setViewport(x: number, y: number, width: number, height: number): void;
    setPickPadding(pickPadding: number): void;
    private update;
    private render;
    private identifyInternal;
    private prepare;
    private getPickData;
    identify(x: number, y: number, camera: Camera | StereoCamera): PickData | undefined;
    asyncIdentify(x: number, y: number, camera: Camera | StereoCamera): AsyncPickData | undefined;
    reset(): void;
    dispose(): void;
    constructor(webgl: WebGLContext, renderer: Renderer, scene: Scene, helper: Helper, pickPass: PickPass, viewport: Viewport, options: PickOptions);
}
