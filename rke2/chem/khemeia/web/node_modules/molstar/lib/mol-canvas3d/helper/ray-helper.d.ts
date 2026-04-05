/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Renderer } from '../../mol-gl/renderer.js';
import { Scene } from '../../mol-gl/scene.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Ray3D } from '../../mol-math/geometry/primitives/ray3d.js';
import { Camera } from '../camera.js';
import { Helper } from './helper.js';
import { AsyncPickData, PickData, PickOptions } from '../passes/pick.js';
import { Sphere3D } from '../../mol-math/geometry/primitives/sphere3d.js';
export declare class RayHelper {
    private webgl;
    private renderer;
    private scene;
    private helper;
    private viewport;
    private size;
    private spiral;
    private pickPadding;
    private camera;
    private pickPass;
    private buffers;
    setPickPadding(pickPadding: number): void;
    private update;
    private render;
    private identifyInternal;
    private prepare;
    private getPickData;
    sphere: Sphere3D;
    private intersectsScene;
    identify(ray: Ray3D, cam: Camera): PickData | undefined;
    asyncIdentify(ray: Ray3D, cam: Camera): AsyncPickData | undefined;
    reset(): void;
    dispose(): void;
    constructor(webgl: WebGLContext, renderer: Renderer, scene: Scene, helper: Helper, options: PickOptions);
}
