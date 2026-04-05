/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { GLRenderingContext } from './compat.js';
import { Framebuffer } from './framebuffer.js';
export type RenderbufferFormat = 'depth16' | 'stencil8' | 'rgba4' | 'depth-stencil' | 'depth24' | 'depth32f' | 'depth24-stencil8' | 'depth32f-stencil8';
export type RenderbufferAttachment = 'depth' | 'stencil' | 'depth-stencil' | 'color0';
export declare function getFormat(gl: GLRenderingContext, format: RenderbufferFormat): 33189 | 36012 | 36168 | 32854 | 34041 | 33190 | 35056 | 36013;
export declare function getAttachment(gl: GLRenderingContext, attachment: RenderbufferAttachment): 36096 | 36128 | 36064 | 33306;
export interface Renderbuffer {
    readonly id: number;
    getByteCount: () => number;
    bind: () => void;
    attachFramebuffer: (framebuffer: Framebuffer) => void;
    detachFramebuffer: (framebuffer: Framebuffer) => void;
    setSize: (width: number, height: number) => void;
    reset: () => void;
    destroy: () => void;
}
export declare function createRenderbuffer(gl: GLRenderingContext, format: RenderbufferFormat, attachment: RenderbufferAttachment, _width: number, _height: number): Renderbuffer;
