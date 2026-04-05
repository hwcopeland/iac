/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ShaderCode, DefineValues } from '../shader-code.js';
import { WebGLState } from './state.js';
import { WebGLExtensions } from './extensions.js';
import { UniformsList, UniformType } from './uniform.js';
import { AttributeBuffers } from './buffer.js';
import { Textures } from './texture.js';
import { RenderableSchema } from '../renderable/schema.js';
import { GLRenderingContext } from './compat.js';
import { ShaderType, Shader } from './shader.js';
import { WebGLParameters } from './context.js';
import { ProgramVariant } from './render-item.js';
export interface Program {
    readonly id: number;
    readonly variant: ProgramVariant;
    isReady(): boolean;
    link(): void;
    finalize(force?: boolean): boolean;
    use: () => void;
    setUniforms: (uniformValues: UniformsList) => void;
    uniform: (k: string, v: UniformType) => void;
    bindAttributes: (attribueBuffers: AttributeBuffers) => void;
    offsetAttributes: (attributeBuffers: AttributeBuffers, offset: number) => void;
    bindTextures: (textures: Textures, startingTargetUnit: number) => void;
    reset: () => void;
    destroy: () => void;
}
export type Programs = {
    [k: string]: Program;
};
export interface ProgramProps {
    defineValues: DefineValues;
    shaderCode: ShaderCode;
    schema: RenderableSchema;
}
export declare function getProgram(gl: GLRenderingContext): WebGLProgram;
type ShaderGetter = (type: ShaderType, source: string) => Shader;
export declare function createProgram(gl: GLRenderingContext, state: WebGLState, extensions: WebGLExtensions, parameters: WebGLParameters, getShader: ShaderGetter, props: ProgramProps): Program;
export {};
