/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { PickingId } from '../../mol-geo/geometry/picking.js';
import { Renderer } from '../../mol-gl/renderer.js';
import { Scene } from '../../mol-gl/scene.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { Vec3 } from '../../mol-math/linear-algebra.js';
import { ICamera } from '../camera.js';
import { Helper } from '../helper/helper.js';
export type PickData = {
    id: PickingId;
    position: Vec3;
};
export type AsyncPickData = {
    tryGet: () => 'pending' | PickData | undefined;
};
export declare const DefaultPickOptions: {
    pickPadding: number;
    maxAsyncReadLag: number;
};
export type PickOptions = typeof DefaultPickOptions;
export declare class PickPass {
    private webgl;
    private width;
    private height;
    private pickScale;
    private readonly objectPickTarget;
    private readonly instancePickTarget;
    private readonly groupPickTarget;
    private readonly depthPickTarget;
    private readonly framebuffer;
    private readonly objectPickTexture;
    private readonly instancePickTexture;
    private readonly groupPickTexture;
    private readonly depthPickTexture;
    private readonly objectPickFramebuffer;
    private readonly instancePickFramebuffer;
    private readonly groupPickFramebuffer;
    private readonly depthPickFramebuffer;
    private readonly depthRenderbuffer;
    private pickWidth;
    private pickHeight;
    constructor(webgl: WebGLContext, width: number, height: number, pickScale: number);
    getByteCount(): number;
    dispose(): void;
    get pickRatio(): number;
    setPickScale(pickScale: number): void;
    bindObject(): void;
    bindInstance(): void;
    bindGroup(): void;
    bindDepth(): void;
    get drawingBufferHeight(): number;
    setSize(width: number, height: number): void;
    reset(): void;
    private renderVariant;
    render(renderer: Renderer, camera: ICamera, scene: Scene, helper: Helper): void;
}
export declare function checkAsyncPickingSupport(webgl: WebGLContext): boolean;
export declare enum AsyncPickStatus {
    Pending = 0,
    Resolved = 1,
    Failed = 2
}
export declare class PickBuffers {
    private webgl;
    private pickPass;
    maxAsyncReadLag: number;
    private object;
    private instance;
    private group;
    private depth;
    private objectBuffer;
    private instanceBuffer;
    private groupBuffer;
    private depthBuffer;
    private viewport;
    private setup;
    setViewport(x: number, y: number, width: number, height: number): void;
    read(): void;
    private fenceSync;
    private fenceTimestamp;
    private ready;
    private lag;
    asyncRead(): void;
    check(): AsyncPickStatus;
    private getIdx;
    getDepth(x: number, y: number): number;
    private getId;
    getObjectId(x: number, y: number): number;
    getInstanceId(x: number, y: number): number;
    getGroupId(x: number, y: number): number;
    getPickingId(x: number, y: number): PickingId | undefined;
    reset(): void;
    dispose(): void;
    constructor(webgl: WebGLContext, pickPass: PickPass, maxAsyncReadLag?: number);
}
