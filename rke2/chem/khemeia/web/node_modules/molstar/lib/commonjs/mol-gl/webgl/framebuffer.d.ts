/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { GLRenderingContext } from './compat.js';
export declare function checkFramebufferStatus(gl: GLRenderingContext, message?: string): void;
export interface Framebuffer {
    readonly id: number;
    bind: () => void;
    reset: () => void;
    destroy: () => void;
}
export declare function createFramebuffer(gl: GLRenderingContext): Framebuffer;
export declare function createNullFramebuffer(): Framebuffer;
