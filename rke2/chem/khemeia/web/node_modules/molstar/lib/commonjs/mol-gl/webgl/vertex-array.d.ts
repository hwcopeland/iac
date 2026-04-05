/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Program } from './program.js';
import { ElementsBuffer, AttributeBuffers } from './buffer.js';
import { WebGLExtensions } from './extensions.js';
import { GLRenderingContext } from './compat.js';
export interface VertexArray {
    readonly id: number;
    bind: () => void;
    update: () => void;
    reset: () => void;
    destroy: () => void;
}
export declare function createVertexArray(gl: GLRenderingContext, extensions: WebGLExtensions, program: Program, attributeBuffers: AttributeBuffers, elementsBuffer?: ElementsBuffer): VertexArray;
