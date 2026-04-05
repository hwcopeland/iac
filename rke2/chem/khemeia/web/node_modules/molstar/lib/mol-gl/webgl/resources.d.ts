/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Program } from './program.js';
import { ShaderType, Shader } from './shader.js';
import { GLRenderingContext } from './compat.js';
import { Framebuffer } from './framebuffer.js';
import { WebGLExtensions } from './extensions.js';
import { WebGLState } from './state.js';
import { AttributeBuffer, UsageHint, ArrayType, AttributeItemSize, ElementsBuffer, ElementsType, AttributeBuffers, PixelPackBuffer } from './buffer.js';
import { WebGLParameters, WebGLStats } from './context.js';
import { DefineValues, ShaderCode } from '../shader-code.js';
import { RenderableSchema } from '../renderable/schema.js';
import { Renderbuffer, RenderbufferAttachment, RenderbufferFormat } from './renderbuffer.js';
import { Texture, TextureKind, TextureFormat, TextureType, TextureFilter, CubeFaces } from './texture.js';
import { VertexArray } from './vertex-array.js';
import { ProgramVariant } from './render-item.js';
type ByteCounts = {
    texture: number;
    cubeTexture: number;
    attribute: number;
    elements: number;
    pixelPack: number;
    renderbuffer: number;
};
export interface WebGLResources {
    attribute: (array: ArrayType, itemSize: AttributeItemSize, divisor: number, usageHint?: UsageHint) => AttributeBuffer;
    elements: (array: ElementsType, usageHint?: UsageHint) => ElementsBuffer;
    pixelPack: (format: TextureFormat, type: TextureType) => PixelPackBuffer;
    framebuffer: () => Framebuffer;
    program: (defineValues: DefineValues, shaderCode: ShaderCode, schema: RenderableSchema) => Program;
    renderbuffer: (format: RenderbufferFormat, attachment: RenderbufferAttachment, width: number, height: number) => Renderbuffer;
    shader: (type: ShaderType, source: string) => Shader;
    texture: (kind: TextureKind, format: TextureFormat, type: TextureType, filter: TextureFilter) => Texture;
    cubeTexture: (faces: CubeFaces, mipmaps: boolean, onload?: () => void) => Texture;
    vertexArray: (program: Program, attributeBuffers: AttributeBuffers, elementsBuffer?: ElementsBuffer) => VertexArray;
    getByteCounts: () => ByteCounts;
    linkPrograms: (variants?: ProgramVariant[]) => void;
    finalizePrograms: (variants?: ProgramVariant[], isSynchronous?: boolean) => boolean;
    reset: () => void;
    destroy: () => void;
}
export declare function createResources(gl: GLRenderingContext, state: WebGLState, stats: WebGLStats, extensions: WebGLExtensions, parameters: WebGLParameters): WebGLResources;
export {};
