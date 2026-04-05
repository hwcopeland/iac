/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { WebGLContext } from './context.js';
import { ValueCell } from '../../mol-util/index.js';
import { RenderableSchema } from '../renderable/schema.js';
import { ValueOf } from '../../mol-util/type-helpers.js';
import { GLRenderingContext } from './compat.js';
import { WebGLExtensions } from './extensions.js';
import { WebGLState } from './state.js';
import { TextureFormat, TextureType } from './texture.js';
export type UsageHint = 'static' | 'dynamic' | 'stream';
export type DataType = 'uint8' | 'int8' | 'uint16' | 'int16' | 'uint32' | 'int32' | 'float32';
export type BufferType = 'attribute' | 'elements' | 'uniform' | 'pixel-pack';
export type DataTypeArrayType = {
    'uint8': Uint8Array;
    'int8': Int8Array;
    'uint16': Uint16Array;
    'int16': Int16Array;
    'uint32': Uint32Array;
    'int32': Int32Array;
    'float32': Float32Array;
};
export type ArrayType = ValueOf<DataTypeArrayType>;
export type ArrayKind = keyof DataTypeArrayType;
export declare function getUsageHint(gl: GLRenderingContext, usageHint: UsageHint): 35044 | 35048 | 35040;
export declare function getDataType(gl: GLRenderingContext, dataType: DataType): 5121 | 5123 | 5126 | 5124 | 5120 | 5122 | 5125;
export declare function getBufferType(gl: GLRenderingContext, bufferType: BufferType): 34962 | 34963 | 35345 | 35051;
export interface Buffer {
    readonly id: number;
    readonly _usageHint: number;
    readonly _bufferType: number;
    readonly _dataType: number;
    readonly _bpe: number;
    readonly length: number;
    getByteCount: () => number;
    getBuffer: () => WebGLBuffer;
    updateData: (array: ArrayType) => void;
    updateSubData: (array: ArrayType, offset: number, count: number) => void;
    reset: () => void;
    destroy: () => void;
}
export declare function getBuffer(gl: GLRenderingContext): WebGLBuffer;
export type AttributeItemSize = 1 | 2 | 3 | 4 | 16;
export type AttributeKind = 'float32';
export declare function getAttribType(gl: GLRenderingContext, kind: AttributeKind, itemSize: AttributeItemSize): 5126 | 35664 | 35665 | 35666 | 35676;
export type AttributeDefs = {
    [k: string]: {
        kind: AttributeKind;
        itemSize: AttributeItemSize;
        divisor: number;
    };
};
export type AttributeValues = {
    [k: string]: ValueCell<ArrayType>;
};
export type AttributeBuffers = [string, AttributeBuffer][];
export interface AttributeBuffer extends Buffer {
    readonly divisor: number;
    bind: (location: number) => void;
    changeOffset: (location: number, offset: number) => void;
}
export declare function createAttributeBuffer<T extends ArrayType, S extends AttributeItemSize>(gl: GLRenderingContext, state: WebGLState, extensions: WebGLExtensions, array: T, itemSize: S, divisor: number, usageHint?: UsageHint): AttributeBuffer;
export declare function createAttributeBuffers(ctx: WebGLContext, schema: RenderableSchema, values: AttributeValues): AttributeBuffers;
export type ElementsType = Uint16Array | Uint32Array;
export type ElementsKind = 'uint16' | 'uint32';
export interface ElementsBuffer extends Buffer {
    bind: () => void;
}
export declare function createElementsBuffer(gl: GLRenderingContext, array: ElementsType, usageHint?: UsageHint): ElementsBuffer;
export interface PixelPackBuffer {
    readonly id: number;
    readonly _type: number;
    readonly _format: number;
    readonly _bpe: number;
    getByteCount: () => number;
    read: (x: number, y: number, width: number, height: number) => void;
    getSubData: (array: ArrayType) => void;
    reset: () => void;
    destroy: () => void;
}
export declare function createPixelPackBuffer(gl: WebGL2RenderingContext, extensions: WebGLExtensions, format: TextureFormat, type: TextureType): PixelPackBuffer;
