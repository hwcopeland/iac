/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Texture, TextureFilter } from './texture.js';
import { Framebuffer } from './framebuffer.js';
import { WebGLResources } from './resources.js';
import { GLRenderingContext } from './compat.js';
import { Renderbuffer } from './renderbuffer.js';
export interface RenderTarget {
    readonly id: number;
    readonly texture: Texture;
    readonly framebuffer: Framebuffer;
    readonly depthRenderbuffer: Renderbuffer | null;
    getByteCount: () => number;
    getWidth: () => number;
    getHeight: () => number;
    /** binds framebuffer */
    bind: () => void;
    setSize: (width: number, height: number) => void;
    reset: () => void;
    destroy: () => void;
}
export declare function createRenderTarget(gl: GLRenderingContext, resources: WebGLResources, _width: number, _height: number, depth?: boolean, type?: 'uint8' | 'float32' | 'fp16', filter?: TextureFilter, format?: 'rgba' | 'alpha'): RenderTarget;
