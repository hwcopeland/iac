/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { idFactory } from '../../mol-util/id-factory.js';
import { assertUnreachable } from '../../mol-util/type-helpers.js';
import { isWebGL2 } from './compat.js';
import { getBytesPerElement, getFormat, getType } from './texture.js';
const getNextBufferId = idFactory();
export function getUsageHint(gl, usageHint) {
    switch (usageHint) {
        case 'static': return gl.STATIC_DRAW;
        case 'dynamic': return gl.DYNAMIC_DRAW;
        case 'stream': return gl.STREAM_DRAW;
        default: assertUnreachable(usageHint);
    }
}
export function getDataType(gl, dataType) {
    switch (dataType) {
        case 'uint8': return gl.UNSIGNED_BYTE;
        case 'int8': return gl.BYTE;
        case 'uint16': return gl.UNSIGNED_SHORT;
        case 'int16': return gl.SHORT;
        case 'uint32': return gl.UNSIGNED_INT;
        case 'int32': return gl.INT;
        case 'float32': return gl.FLOAT;
        default: assertUnreachable(dataType);
    }
}
function dataTypeFromArray(gl, array) {
    if (array instanceof Uint8Array) {
        return gl.UNSIGNED_BYTE;
    }
    else if (array instanceof Int8Array) {
        return gl.BYTE;
    }
    else if (array instanceof Uint16Array) {
        return gl.UNSIGNED_SHORT;
    }
    else if (array instanceof Int16Array) {
        return gl.SHORT;
    }
    else if (array instanceof Uint32Array) {
        return gl.UNSIGNED_INT;
    }
    else if (array instanceof Int32Array) {
        return gl.INT;
    }
    else if (array instanceof Float32Array) {
        return gl.FLOAT;
    }
    assertUnreachable(array);
}
export function getBufferType(gl, bufferType) {
    switch (bufferType) {
        case 'attribute': return gl.ARRAY_BUFFER;
        case 'elements': return gl.ELEMENT_ARRAY_BUFFER;
        case 'uniform':
            if (isWebGL2(gl)) {
                return gl.UNIFORM_BUFFER;
            }
            else {
                throw new Error('WebGL2 is required for uniform buffers');
            }
        case 'pixel-pack':
            if (isWebGL2(gl)) {
                return gl.PIXEL_PACK_BUFFER;
            }
            else {
                throw new Error('WebGL2 is required for pixel-pack buffers');
            }
    }
}
export function getBuffer(gl) {
    const buffer = gl.createBuffer();
    if (buffer === null) {
        throw new Error('Could not create WebGL buffer');
    }
    return buffer;
}
function createBuffer(gl, array, usageHint, bufferType) {
    let _buffer = getBuffer(gl);
    const _usageHint = getUsageHint(gl, usageHint);
    const _bufferType = getBufferType(gl, bufferType);
    const _dataType = dataTypeFromArray(gl, array);
    const _bpe = array.BYTES_PER_ELEMENT;
    const _length = array.length;
    function updateData(array) {
        gl.bindBuffer(_bufferType, _buffer);
        gl.bufferData(_bufferType, array, _usageHint);
    }
    updateData(array);
    let destroyed = false;
    return {
        id: getNextBufferId(),
        _usageHint,
        _bufferType,
        _dataType,
        _bpe,
        length: _length,
        getByteCount: () => {
            return _bpe * _length;
        },
        getBuffer: () => _buffer,
        updateData,
        updateSubData: (array, offset, count) => {
            gl.bindBuffer(_bufferType, _buffer);
            if (count - offset === array.length) {
                gl.bufferSubData(_bufferType, 0, array);
            }
            else {
                gl.bufferSubData(_bufferType, offset * _bpe, array.subarray(offset, offset + count));
            }
        },
        reset: () => {
            _buffer = getBuffer(gl);
            updateData(array);
        },
        destroy: () => {
            if (destroyed)
                return;
            gl.deleteBuffer(_buffer);
            destroyed = true;
        }
    };
}
export function getAttribType(gl, kind, itemSize) {
    switch (kind) {
        case 'float32':
            switch (itemSize) {
                case 1: return gl.FLOAT;
                case 2: return gl.FLOAT_VEC2;
                case 3: return gl.FLOAT_VEC3;
                case 4: return gl.FLOAT_VEC4;
                case 16: return gl.FLOAT_MAT4;
            }
        default:
            assertUnreachable(kind);
    }
}
export function createAttributeBuffer(gl, state, extensions, array, itemSize, divisor, usageHint = 'static') {
    const { instancedArrays } = extensions;
    const buffer = createBuffer(gl, array, usageHint, 'attribute');
    const { _bufferType, _dataType, _bpe } = buffer;
    return {
        ...buffer,
        divisor,
        bind: (location) => {
            gl.bindBuffer(_bufferType, buffer.getBuffer());
            if (itemSize === 16) {
                for (let i = 0; i < 4; ++i) {
                    state.enableVertexAttrib(location + i);
                    gl.vertexAttribPointer(location + i, 4, _dataType, false, 4 * 4 * _bpe, i * 4 * _bpe);
                    instancedArrays.vertexAttribDivisor(location + i, divisor);
                }
            }
            else {
                state.enableVertexAttrib(location);
                gl.vertexAttribPointer(location, itemSize, _dataType, false, 0, 0);
                instancedArrays.vertexAttribDivisor(location, divisor);
            }
        },
        changeOffset: (location, offset) => {
            const o = offset * _bpe * itemSize;
            gl.bindBuffer(_bufferType, buffer.getBuffer());
            if (itemSize === 16) {
                for (let i = 0; i < 4; ++i) {
                    gl.vertexAttribPointer(location + i, 4, _dataType, false, 4 * 4 * _bpe, (i * 4 * _bpe) + o);
                }
            }
            else {
                gl.vertexAttribPointer(location, itemSize, _dataType, false, 0, o);
            }
        }
    };
}
export function createAttributeBuffers(ctx, schema, values) {
    const buffers = [];
    Object.keys(schema).forEach(k => {
        const spec = schema[k];
        if (spec.type === 'attribute') {
            buffers[buffers.length] = [k, ctx.resources.attribute(values[k].ref.value, spec.itemSize, spec.divisor)];
        }
    });
    return buffers;
}
export function createElementsBuffer(gl, array, usageHint = 'static') {
    const buffer = createBuffer(gl, array, usageHint, 'elements');
    return {
        ...buffer,
        bind: () => {
            gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, buffer.getBuffer());
        }
    };
}
export function createPixelPackBuffer(gl, extensions, format, type) {
    let _buffer = getBuffer(gl);
    const _type = getType(gl, extensions, type);
    const _format = getFormat(gl, format, type);
    const _bpe = getBytesPerElement(format, type);
    let _width = 0;
    let _height = 0;
    function read(x, y, width, height) {
        _width = width;
        _height = height;
        gl.bindBuffer(gl.PIXEL_PACK_BUFFER, _buffer);
        gl.bufferData(gl.PIXEL_PACK_BUFFER, width * height * _bpe, gl.STREAM_READ);
        gl.readPixels(x, y, width, height, _format, _type, 0);
        gl.bindBuffer(gl.PIXEL_PACK_BUFFER, null);
    }
    function getSubData(array) {
        gl.bindBuffer(gl.PIXEL_PACK_BUFFER, _buffer);
        gl.getBufferSubData(gl.PIXEL_PACK_BUFFER, 0, array);
        gl.bindBuffer(gl.PIXEL_PACK_BUFFER, null);
    }
    let destroyed = false;
    return {
        id: getNextBufferId(),
        _type,
        _format,
        _bpe,
        getByteCount: () => {
            return _bpe * _width * _height;
        },
        read,
        getSubData,
        reset: () => {
            _buffer = getBuffer(gl);
        },
        destroy: () => {
            if (destroyed)
                return;
            gl.deleteBuffer(_buffer);
            destroyed = true;
        }
    };
}
