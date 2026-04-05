/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { GLRenderingContext } from './compat.js';
import { Framebuffer } from './framebuffer.js';
import { WebGLExtensions } from './extensions.js';
import { WebGLState } from './state.js';
import { WebGLResources } from './resources.js';
import { RenderTarget } from './render-target.js';
import { Subject } from 'rxjs';
import { now } from '../../mol-util/now.js';
import { Texture, TextureFilter } from './texture.js';
import { ComputeRenderable } from '../renderable.js';
import { WebGLTimer } from './timer.js';
export declare function getGLContext(canvas: HTMLCanvasElement, attribs?: WebGLContextAttributes & {
    preferWebGl1?: boolean;
}): GLRenderingContext | null;
export declare function getErrorDescription(gl: GLRenderingContext, error: number): "unknown error" | "no error" | "invalid enum" | "invalid value" | "invalid operation" | "invalid framebuffer operation" | "out of memory" | "context lost";
export declare function checkError(gl: GLRenderingContext, message?: string): void;
export declare function glEnumToString(gl: GLRenderingContext, value: number): string;
export declare function readPixels(gl: GLRenderingContext, x: number, y: number, width: number, height: number, buffer: Uint8Array | Float32Array | Int32Array): void;
declare function getShaderPrecisionFormats(gl: GLRenderingContext, shader: 'vertex' | 'fragment'): {
    lowFloat: WebGLShaderPrecisionFormat | null;
    mediumFloat: WebGLShaderPrecisionFormat | null;
    highFloat: WebGLShaderPrecisionFormat | null;
    lowInt: WebGLShaderPrecisionFormat | null;
    mediumInt: WebGLShaderPrecisionFormat | null;
    highInt: WebGLShaderPrecisionFormat | null;
};
type WebGLShaderPrecisionFormats = ReturnType<typeof getShaderPrecisionFormats>;
declare function createStats(): {
    resourceCounts: {
        attribute: number;
        elements: number;
        pixelPack: number;
        framebuffer: number;
        program: number;
        renderbuffer: number;
        shader: number;
        texture: number;
        cubeTexture: number;
        vertexArray: number;
    };
    drawCount: number;
    instanceCount: number;
    instancedDrawCount: number;
    calls: {
        drawInstanced: number;
        drawInstancedBase: number;
        multiDrawInstancedBase: number;
        counts: number;
    };
    culled: {
        lod: number;
        frustum: number;
        occlusion: number;
    };
};
export type WebGLStats = ReturnType<typeof createStats>;
declare function createParameters(gl: GLRenderingContext, extensions: WebGLExtensions): {
    maxTextureSize: number;
    max3dTextureSize: number;
    maxRenderbufferSize: number;
    maxDrawBuffers: number;
    maxTextureImageUnits: number;
    maxVertexTextureImageUnits: number;
};
export type WebGLParameters = ReturnType<typeof createParameters>;
/** A WebGL context object, including the rendering context, resource caches and counts */
export interface WebGLContext {
    readonly gl: GLRenderingContext;
    readonly isWebGL2: boolean;
    readonly pixelRatio: number;
    readonly extensions: WebGLExtensions;
    readonly state: WebGLState;
    readonly stats: WebGLStats;
    readonly resources: WebGLResources;
    readonly timer: WebGLTimer;
    readonly maxTextureSize: number;
    readonly max3dTextureSize: number;
    readonly maxRenderbufferSize: number;
    readonly maxDrawBuffers: number;
    readonly maxTextureImageUnits: number;
    readonly shaderPrecisionFormats: {
        vertex: WebGLShaderPrecisionFormats;
        fragment: WebGLShaderPrecisionFormats;
    };
    readonly isContextLost: boolean;
    readonly contextRestored: Subject<now.Timestamp>;
    setContextLost: () => void;
    handleContextRestored: (extraResets?: () => void) => void;
    readonly xr: {
        readonly session?: XRSession;
        readonly changed: Subject<void>;
        set: (session: XRSession | undefined, options?: {
            resolutionScale?: number;
        }) => Promise<void>;
        end: () => Promise<void>;
    };
    setPixelScale: (value: number) => void;
    /** Cache for compute renderables, managed by consumers */
    readonly namedComputeRenderables: {
        [name: string]: ComputeRenderable<any>;
    };
    /** Cache for frambuffers, managed by consumers */
    readonly namedFramebuffers: {
        [name: string]: Framebuffer;
    };
    /** Cache for textures, managed by consumers */
    readonly namedTextures: {
        [name: string]: Texture;
    };
    createRenderTarget: (width: number, height: number, depth?: boolean, type?: 'uint8' | 'float32' | 'fp16', filter?: TextureFilter, format?: 'rgba' | 'alpha') => RenderTarget;
    createDrawTarget: () => RenderTarget;
    bindDrawingBuffer: () => void;
    getDrawingBufferSize: () => {
        width: number;
        height: number;
    };
    readPixels: (x: number, y: number, width: number, height: number, buffer: Uint8Array | Float32Array | Int32Array) => void;
    waitForGpuCommandsComplete: () => Promise<void>;
    waitForGpuCommandsCompleteSync: () => void;
    getFenceSync: () => WebGLSync | null;
    checkSyncStatus: (sync: WebGLSync) => boolean;
    deleteSync: (sync: WebGLSync) => void;
    clear: (red: number, green: number, blue: number, alpha: number) => void;
    checkError: (message?: string) => void;
    checkFramebufferStatus: (message?: string) => void;
    destroy: (options?: Partial<{
        doNotForceWebGLContextLoss: boolean;
    }>) => void;
}
export declare function createContext(gl: GLRenderingContext, props?: Partial<{
    pixelScale: number;
}>): WebGLContext;
export {};
