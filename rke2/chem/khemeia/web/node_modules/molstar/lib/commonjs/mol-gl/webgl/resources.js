"use strict";
/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.createResources = createResources;
const program_1 = require("./program.js");
const shader_1 = require("./shader.js");
const compat_1 = require("./compat.js");
const framebuffer_1 = require("./framebuffer.js");
const buffer_1 = require("./buffer.js");
const reference_cache_1 = require("../../mol-util/reference-cache.js");
const util_1 = require("../../mol-data/util.js");
const renderbuffer_1 = require("./renderbuffer.js");
const texture_1 = require("./texture.js");
const vertex_array_1 = require("./vertex-array.js");
const now_1 = require("../../mol-util/now.js");
const debug_1 = require("../../mol-util/debug.js");
function defineValueHash(v) {
    return typeof v === 'boolean' ? (v ? 1 : 0) :
        typeof v === 'number' ? (v * 10000) : (0, util_1.hashString)(v);
}
function wrapCached(resourceItem) {
    const wrapped = {
        ...resourceItem.value,
        destroy: () => {
            resourceItem.free();
        }
    };
    return wrapped;
}
function createResources(gl, state, stats, extensions, parameters) {
    const sets = {
        attribute: new Set(),
        elements: new Set(),
        pixelPack: new Set(),
        framebuffer: new Set(),
        program: new Set(),
        renderbuffer: new Set(),
        shader: new Set(),
        texture: new Set(),
        cubeTexture: new Set(),
        vertexArray: new Set(),
    };
    function wrap(name, resource) {
        sets[name].add(resource);
        stats.resourceCounts[name] += 1;
        return {
            ...resource,
            destroy: () => {
                resource.destroy();
                sets[name].delete(resource);
                stats.resourceCounts[name] -= 1;
            }
        };
    }
    const shaderCache = (0, reference_cache_1.createReferenceCache)((props) => JSON.stringify(props), (props) => wrap('shader', (0, shader_1.createShader)(gl, props)), (shader) => { shader.destroy(); });
    function getShader(type, source) {
        return wrapCached(shaderCache.get({ type, source }));
    }
    const pendingPrograms = new Set();
    const programCache = (0, reference_cache_1.createReferenceCache)((props) => {
        var _a;
        const array = [props.shaderCode.id];
        const variant = (((_a = props.defineValues.dRenderVariant) === null || _a === void 0 ? void 0 : _a.ref.value) || '');
        Object.keys(props.defineValues).forEach(k => {
            var _a, _b;
            if (!((_b = (_a = props.shaderCode).ignoreDefine) === null || _b === void 0 ? void 0 : _b.call(_a, k, variant, props.defineValues))) {
                array.push((0, util_1.hashString)(k), defineValueHash(props.defineValues[k].ref.value));
            }
        });
        return (0, util_1.hashFnv32a)(array).toString();
    }, (props) => {
        const program = (0, program_1.createProgram)(gl, state, extensions, parameters, getShader, props);
        if (program.variant !== 'compute') {
            pendingPrograms.add(program);
        }
        return wrap('program', program);
    }, (program) => {
        pendingPrograms.delete(program);
        program.destroy();
    });
    return {
        attribute: (array, itemSize, divisor, usageHint) => {
            return wrap('attribute', (0, buffer_1.createAttributeBuffer)(gl, state, extensions, array, itemSize, divisor, usageHint));
        },
        elements: (array, usageHint) => {
            return wrap('elements', (0, buffer_1.createElementsBuffer)(gl, array, usageHint));
        },
        pixelPack: (format, type) => {
            if (!(0, compat_1.isWebGL2)(gl)) {
                throw new Error('WebGL2 is required for pixel-pack buffers');
            }
            return wrap('pixelPack', (0, buffer_1.createPixelPackBuffer)(gl, extensions, format, type));
        },
        framebuffer: () => {
            return wrap('framebuffer', (0, framebuffer_1.createFramebuffer)(gl));
        },
        program: (defineValues, shaderCode, schema) => {
            return wrapCached(programCache.get({ defineValues, shaderCode, schema }));
        },
        renderbuffer: (format, attachment, width, height) => {
            return wrap('renderbuffer', (0, renderbuffer_1.createRenderbuffer)(gl, format, attachment, width, height));
        },
        shader: getShader,
        texture: (kind, format, type, filter) => {
            return wrap('texture', (0, texture_1.createTexture)(gl, extensions, kind, format, type, filter));
        },
        cubeTexture: (faces, mipmaps, onload) => {
            return wrap('cubeTexture', (0, texture_1.createCubeTexture)(gl, faces, mipmaps, onload));
        },
        vertexArray: (program, attributeBuffers, elementsBuffer) => {
            return wrap('vertexArray', (0, vertex_array_1.createVertexArray)(gl, extensions, program, attributeBuffers, elementsBuffer));
        },
        getByteCounts: () => {
            let texture = 0;
            sets.texture.forEach(r => {
                texture += r.getByteCount();
            });
            let cubeTexture = 0;
            sets.cubeTexture.forEach(r => {
                cubeTexture += r.getByteCount();
            });
            let attribute = 0;
            sets.attribute.forEach(r => {
                attribute += r.getByteCount();
            });
            let elements = 0;
            sets.elements.forEach(r => {
                elements += r.getByteCount();
            });
            let pixelPack = 0;
            sets.pixelPack.forEach(r => {
                pixelPack += r.getByteCount();
            });
            let renderbuffer = 0;
            sets.renderbuffer.forEach(r => {
                renderbuffer += r.getByteCount();
            });
            return { texture, cubeTexture, attribute, elements, pixelPack, renderbuffer };
        },
        linkPrograms: (variants) => {
            for (const p of pendingPrograms) {
                if (variants && !variants.includes(p.variant))
                    continue;
                p.link();
            }
        },
        finalizePrograms: (variants, isSynchronous) => {
            let isReady = true;
            let pendingCount = 0;
            for (const p of pendingPrograms) {
                if (p.isReady())
                    pendingPrograms.delete(p);
                if (!variants || variants.includes(p.variant)) {
                    isReady = false;
                    pendingCount += 1;
                }
            }
            if (isReady)
                return true;
            let linkStatus = true;
            let finalizedCount = 0;
            const t = (0, now_1.now)();
            for (const p of pendingPrograms) {
                if (variants && !variants.includes(p.variant))
                    continue;
                if (!p.finalize(isSynchronous)) {
                    linkStatus = false;
                }
                else {
                    pendingPrograms.delete(p);
                    finalizedCount += 1;
                }
                if (!isSynchronous && (0, now_1.now)() - t > 16) {
                    linkStatus = false;
                    break;
                }
            }
            if (debug_1.isTimingMode) {
                console.log(`Finalized ${finalizedCount} of ${pendingCount} programs (${variants ? variants.join(', ') : 'all'}) in ${((0, now_1.now)() - t).toFixed(2)} ms`);
            }
            return linkStatus;
        },
        reset: () => {
            sets.attribute.forEach(r => r.reset());
            sets.elements.forEach(r => r.reset());
            sets.pixelPack.forEach(r => r.reset());
            sets.framebuffer.forEach(r => r.reset());
            sets.renderbuffer.forEach(r => r.reset());
            sets.shader.forEach(r => r.reset());
            sets.program.forEach(r => r.reset());
            sets.vertexArray.forEach(r => r.reset());
            sets.texture.forEach(r => r.reset());
            sets.cubeTexture.forEach(r => r.reset());
        },
        destroy: () => {
            sets.attribute.forEach(r => r.destroy());
            sets.elements.forEach(r => r.destroy());
            sets.pixelPack.forEach(r => r.destroy());
            sets.framebuffer.forEach(r => r.destroy());
            sets.renderbuffer.forEach(r => r.destroy());
            sets.shader.forEach(r => r.destroy());
            sets.program.forEach(r => r.destroy());
            sets.vertexArray.forEach(r => r.destroy());
            sets.texture.forEach(r => r.destroy());
            sets.cubeTexture.forEach(r => r.destroy());
            shaderCache.clear();
            programCache.clear();
            pendingPrograms.clear();
        }
    };
}
