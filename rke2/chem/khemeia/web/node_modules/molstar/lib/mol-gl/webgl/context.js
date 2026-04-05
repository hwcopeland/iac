/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { isWebGL2 } from './compat.js';
import { checkFramebufferStatus, createNullFramebuffer } from './framebuffer.js';
import { Scheduler } from '../../mol-task/index.js';
import { isDebugMode } from '../../mol-util/debug.js';
import { createExtensions, resetExtensions } from './extensions.js';
import { createState } from './state.js';
import { createResources } from './resources.js';
import { createRenderTarget } from './render-target.js';
import { Subject } from 'rxjs';
import { now } from '../../mol-util/now.js';
import { createNullTexture } from './texture.js';
import { createTimer } from './timer.js';
export function getGLContext(canvas, attribs) {
    function get(id) {
        try {
            return canvas.getContext(id, attribs);
        }
        catch (e) {
            return null;
        }
    }
    const gl = ((attribs === null || attribs === void 0 ? void 0 : attribs.preferWebGl1) ? null : get('webgl2')) || get('webgl') || get('experimental-webgl');
    if (isDebugMode)
        console.log(`isWebgl2: ${isWebGL2(gl)}`);
    return gl;
}
export function getErrorDescription(gl, error) {
    switch (error) {
        case gl.NO_ERROR: return 'no error';
        case gl.INVALID_ENUM: return 'invalid enum';
        case gl.INVALID_VALUE: return 'invalid value';
        case gl.INVALID_OPERATION: return 'invalid operation';
        case gl.INVALID_FRAMEBUFFER_OPERATION: return 'invalid framebuffer operation';
        case gl.OUT_OF_MEMORY: return 'out of memory';
        case gl.CONTEXT_LOST_WEBGL: return 'context lost';
    }
    return 'unknown error';
}
export function checkError(gl, message) {
    const error = gl.getError();
    if (error !== gl.NO_ERROR) {
        console.log(`WebGL error: '${getErrorDescription(gl, error)}'${message ? ` (${message})` : ''}`);
        // throw new Error(`WebGL error: '${getErrorDescription(gl, error)}'${message ? ` (${message})` : ''}`);
    }
}
export function glEnumToString(gl, value) {
    const keys = [];
    for (const key in gl) {
        if (gl[key] === value) {
            keys.push(key);
        }
    }
    return keys.length ? keys.join(' | ') : `0x${value.toString(16)}`;
}
function unbindResources(gl) {
    // bind null to all texture units
    const maxTextureImageUnits = gl.getParameter(gl.MAX_TEXTURE_IMAGE_UNITS);
    for (let i = 0; i < maxTextureImageUnits; ++i) {
        gl.activeTexture(gl.TEXTURE0 + i);
        gl.bindTexture(gl.TEXTURE_2D, null);
        gl.bindTexture(gl.TEXTURE_CUBE_MAP, null);
        if (isWebGL2(gl)) {
            gl.bindTexture(gl.TEXTURE_2D_ARRAY, null);
            gl.bindTexture(gl.TEXTURE_3D, null);
        }
    }
    // assign the smallest possible buffer to all attributes
    const buf = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    const maxVertexAttribs = gl.getParameter(gl.MAX_VERTEX_ATTRIBS);
    for (let i = 0; i < maxVertexAttribs; ++i) {
        gl.vertexAttribPointer(i, 1, gl.FLOAT, false, 0, 0);
    }
    // bind null to all buffers
    gl.bindBuffer(gl.ARRAY_BUFFER, null);
    gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, null);
    gl.bindRenderbuffer(gl.RENDERBUFFER, null);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    if (isWebGL2(gl)) {
        gl.bindBuffer(gl.UNIFORM_BUFFER, null);
        gl.bindBuffer(gl.PIXEL_PACK_BUFFER, null);
    }
}
const tmpPixel = new Uint8Array(1 * 4);
function checkSync(gl, sync, resolve) {
    if (gl.getSyncParameter(sync, gl.SYNC_STATUS) === gl.SIGNALED) {
        gl.deleteSync(sync);
        resolve();
    }
    else {
        Scheduler.setImmediate(checkSync, gl, sync, resolve);
    }
}
function fence(gl, resolve) {
    const sync = gl.fenceSync(gl.SYNC_GPU_COMMANDS_COMPLETE, 0);
    if (!sync) {
        console.warn('Could not create a WebGLSync object');
        gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, tmpPixel);
        resolve();
    }
    else {
        Scheduler.setImmediate(checkSync, gl, sync, resolve);
    }
}
let SentWebglSyncObjectNotSupportedInWebglMessage = false;
function waitForGpuCommandsComplete(gl) {
    return new Promise(resolve => {
        if (isWebGL2(gl)) {
            fence(gl, resolve);
        }
        else {
            if (!SentWebglSyncObjectNotSupportedInWebglMessage) {
                console.info('Sync object not supported in WebGL');
                SentWebglSyncObjectNotSupportedInWebglMessage = true;
            }
            waitForGpuCommandsCompleteSync(gl);
            resolve();
        }
    });
}
function waitForGpuCommandsCompleteSync(gl) {
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, tmpPixel);
}
export function readPixels(gl, x, y, width, height, buffer) {
    if (isDebugMode)
        checkFramebufferStatus(gl);
    if (buffer instanceof Uint8Array) {
        gl.readPixels(x, y, width, height, gl.RGBA, gl.UNSIGNED_BYTE, buffer);
    }
    else if (buffer instanceof Float32Array) {
        gl.readPixels(x, y, width, height, gl.RGBA, gl.FLOAT, buffer);
    }
    else if (buffer instanceof Int32Array && isWebGL2(gl)) {
        gl.readPixels(x, y, width, height, gl.RGBA_INTEGER, gl.INT, buffer);
    }
    else {
        throw new Error('unsupported readPixels buffer type');
    }
    if (isDebugMode)
        checkError(gl);
}
function bindDrawingBuffer(gl, xrLayer) {
    if (xrLayer) {
        gl.bindFramebuffer(gl.FRAMEBUFFER, xrLayer.framebuffer);
    }
    else {
        gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    }
}
function getDrawingBufferSize(gl, xrLayer, xrInteractionMode) {
    var _a, _b;
    let width = (_a = xrLayer === null || xrLayer === void 0 ? void 0 : xrLayer.framebufferWidth) !== null && _a !== void 0 ? _a : gl.drawingBufferWidth;
    if (xrInteractionMode === 'screen-space') {
        // workaround so XR with a single view behaves simlar to two views
        width *= 2;
    }
    const height = (_b = xrLayer === null || xrLayer === void 0 ? void 0 : xrLayer.framebufferHeight) !== null && _b !== void 0 ? _b : gl.drawingBufferHeight;
    return { width, height };
}
function getShaderPrecisionFormat(gl, shader, precision, type) {
    const glShader = shader === 'vertex' ? gl.VERTEX_SHADER : gl.FRAGMENT_SHADER;
    const glPrecisionType = gl[`${precision.toUpperCase()}_${type.toUpperCase()}`];
    return gl.getShaderPrecisionFormat(glShader, glPrecisionType);
}
function getShaderPrecisionFormats(gl, shader) {
    return {
        lowFloat: getShaderPrecisionFormat(gl, shader, 'low', 'float'),
        mediumFloat: getShaderPrecisionFormat(gl, shader, 'medium', 'float'),
        highFloat: getShaderPrecisionFormat(gl, shader, 'high', 'float'),
        lowInt: getShaderPrecisionFormat(gl, shader, 'low', 'int'),
        mediumInt: getShaderPrecisionFormat(gl, shader, 'medium', 'int'),
        highInt: getShaderPrecisionFormat(gl, shader, 'high', 'int'),
    };
}
//
function createStats() {
    const stats = {
        resourceCounts: {
            attribute: 0,
            elements: 0,
            pixelPack: 0,
            framebuffer: 0,
            program: 0,
            renderbuffer: 0,
            shader: 0,
            texture: 0,
            cubeTexture: 0,
            vertexArray: 0,
        },
        drawCount: 0,
        instanceCount: 0,
        instancedDrawCount: 0,
        calls: {
            drawInstanced: 0,
            drawInstancedBase: 0,
            multiDrawInstancedBase: 0,
            counts: 0,
        },
        culled: {
            lod: 0,
            frustum: 0,
            occlusion: 0,
        },
    };
    return stats;
}
//
function createParameters(gl, extensions) {
    return {
        maxTextureSize: gl.getParameter(gl.MAX_TEXTURE_SIZE),
        max3dTextureSize: isWebGL2(gl) ? gl.getParameter(gl.MAX_3D_TEXTURE_SIZE) : 0,
        maxRenderbufferSize: gl.getParameter(gl.MAX_RENDERBUFFER_SIZE),
        maxDrawBuffers: extensions.drawBuffers ? gl.getParameter(extensions.drawBuffers.MAX_DRAW_BUFFERS) : 0,
        maxTextureImageUnits: gl.getParameter(gl.MAX_TEXTURE_IMAGE_UNITS),
        maxVertexTextureImageUnits: gl.getParameter(gl.MAX_VERTEX_TEXTURE_IMAGE_UNITS),
    };
}
export function createContext(gl, props = {}) {
    const extensions = createExtensions(gl);
    const state = createState(gl, extensions);
    const stats = createStats();
    const parameters = createParameters(gl, extensions);
    const resources = createResources(gl, state, stats, extensions, parameters);
    const timer = createTimer(gl, extensions, stats);
    if (parameters.maxVertexTextureImageUnits < 8) {
        throw new Error('Need "MAX_VERTEX_TEXTURE_IMAGE_UNITS" >= 8');
    }
    const shaderPrecisionFormats = {
        vertex: getShaderPrecisionFormats(gl, 'vertex'),
        fragment: getShaderPrecisionFormats(gl, 'fragment'),
    };
    if (isDebugMode) {
        console.log({ parameters, shaderPrecisionFormats });
    }
    // optimize assuming flats first and last data are same or differences don't matter
    // extension is only available when `FIRST_VERTEX_CONVENTION` is more efficient
    const epv = extensions.provokingVertex;
    epv === null || epv === void 0 ? void 0 : epv.provokingVertex(epv.FIRST_VERTEX_CONVENTION);
    let isContextLost = false;
    const contextRestored = new Subject();
    let pixelScale = props.pixelScale || 1;
    const xr = {
        session: undefined,
        layer: undefined,
        changed: new Subject(),
        clear: () => {
            xr.layer = undefined;
            xr.session = undefined;
            xr.changed.next();
        }
    };
    const renderTargets = new Set();
    return {
        gl,
        isWebGL2: isWebGL2(gl),
        get pixelRatio() {
            const dpr = (typeof window !== 'undefined') ? (window.devicePixelRatio || 1) : 1;
            return dpr * (pixelScale || 1);
        },
        extensions,
        state,
        stats,
        resources,
        timer,
        get maxTextureSize() { return parameters.maxTextureSize; },
        get max3dTextureSize() { return parameters.max3dTextureSize; },
        get maxRenderbufferSize() { return parameters.maxRenderbufferSize; },
        get maxDrawBuffers() { return parameters.maxDrawBuffers; },
        get maxTextureImageUnits() { return parameters.maxTextureImageUnits; },
        get shaderPrecisionFormats() { return shaderPrecisionFormats; },
        namedComputeRenderables: Object.create(null),
        namedFramebuffers: Object.create(null),
        namedTextures: Object.create(null),
        get isContextLost() {
            return isContextLost || gl.isContextLost();
        },
        contextRestored,
        setContextLost: () => {
            isContextLost = true;
            timer.clear();
        },
        handleContextRestored: (extraResets) => {
            resetExtensions(gl, extensions);
            state.reset();
            state.currentMaterialId = -1;
            state.currentProgramId = -1;
            state.currentRenderItemId = -1;
            resources.reset();
            renderTargets.forEach(rt => rt.reset());
            extraResets === null || extraResets === void 0 ? void 0 : extraResets();
            isContextLost = false;
            contextRestored.next(now());
        },
        xr: {
            get session() {
                return xr.session;
            },
            changed: xr.changed,
            set: async (session, options) => {
                var _a, _b;
                if (xr.session === session)
                    return;
                await ((_a = xr.session) === null || _a === void 0 ? void 0 : _a.end());
                if (session === undefined)
                    return;
                try {
                    await gl.makeXRCompatible();
                    xr.session = session;
                    xr.layer = new XRWebGLLayer(xr.session, gl, {
                        antialias: true,
                        alpha: true,
                        depth: true,
                        framebufferScaleFactor: pixelScale * ((_b = options === null || options === void 0 ? void 0 : options.resolutionScale) !== null && _b !== void 0 ? _b : 1),
                    });
                    await xr.session.updateRenderState({ baseLayer: xr.layer });
                    xr.session.addEventListener('end', xr.clear);
                    xr.changed.next();
                }
                catch (err) {
                    if (session) {
                        await session.end();
                    }
                    else {
                        xr.layer = undefined;
                        xr.session = undefined;
                    }
                    throw err;
                }
            },
            end: async () => {
                var _a;
                return (_a = xr.session) === null || _a === void 0 ? void 0 : _a.end();
            },
        },
        setPixelScale: (value) => {
            pixelScale = value;
        },
        createRenderTarget: (width, height, depth, type, filter, format) => {
            const renderTarget = createRenderTarget(gl, resources, width, height, depth, type, filter, format);
            renderTargets.add(renderTarget);
            return {
                ...renderTarget,
                destroy: () => {
                    renderTarget.destroy();
                    renderTargets.delete(renderTarget);
                }
            };
        },
        createDrawTarget: () => {
            return {
                id: -1,
                texture: createNullTexture(gl),
                framebuffer: createNullFramebuffer(),
                depthRenderbuffer: null,
                getByteCount: () => 0,
                getWidth: () => { var _a; return getDrawingBufferSize(gl, xr.layer, (_a = xr.session) === null || _a === void 0 ? void 0 : _a.interactionMode).width; },
                getHeight: () => { var _a; return getDrawingBufferSize(gl, xr.layer, (_a = xr.session) === null || _a === void 0 ? void 0 : _a.interactionMode).height; },
                bind: () => {
                    bindDrawingBuffer(gl, xr.layer);
                },
                setSize: () => { },
                reset: () => { },
                destroy: () => { }
            };
        },
        bindDrawingBuffer: () => bindDrawingBuffer(gl, xr.layer),
        getDrawingBufferSize: () => { var _a; return getDrawingBufferSize(gl, xr.layer, (_a = xr.session) === null || _a === void 0 ? void 0 : _a.interactionMode); },
        readPixels: (x, y, width, height, buffer) => {
            readPixels(gl, x, y, width, height, buffer);
        },
        waitForGpuCommandsComplete: () => waitForGpuCommandsComplete(gl),
        waitForGpuCommandsCompleteSync: () => waitForGpuCommandsCompleteSync(gl),
        getFenceSync: () => {
            return isWebGL2(gl) ? gl.fenceSync(gl.SYNC_GPU_COMMANDS_COMPLETE, 0) : null;
        },
        checkSyncStatus: (sync) => {
            if (!isWebGL2(gl))
                return true;
            if (gl.getSyncParameter(sync, gl.SYNC_STATUS) === gl.SIGNALED) {
                gl.deleteSync(sync);
                return true;
            }
            else {
                return false;
            }
        },
        deleteSync: (sync) => {
            if (isWebGL2(gl))
                gl.deleteSync(sync);
        },
        clear: (red, green, blue, alpha) => {
            const drs = getDrawingBufferSize(gl, xr.layer);
            bindDrawingBuffer(gl, xr.layer);
            state.enable(gl.SCISSOR_TEST);
            state.depthMask(true);
            state.colorMask(true, true, true, true);
            state.clearColor(red, green, blue, alpha);
            state.viewport(0, 0, drs.width, drs.height);
            state.scissor(0, 0, drs.width, drs.height);
            gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
        },
        checkError: (message) => {
            checkError(gl, message);
        },
        checkFramebufferStatus: (message) => {
            checkFramebufferStatus(gl, message);
        },
        destroy: (options) => {
            var _a, _b, _c, _d;
            resources.destroy();
            unbindResources(gl);
            (_a = xr.session) === null || _a === void 0 ? void 0 : _a.removeEventListener('end', xr.clear);
            (_b = xr.session) === null || _b === void 0 ? void 0 : _b.end();
            contextRestored.complete();
            xr.changed.complete();
            // to aid GC
            if (!(options === null || options === void 0 ? void 0 : options.doNotForceWebGLContextLoss)) {
                (_c = gl.getExtension('WEBGL_lose_context')) === null || _c === void 0 ? void 0 : _c.loseContext();
                (_d = gl.getExtension('STACKGL_destroy_context')) === null || _d === void 0 ? void 0 : _d.destroy();
            }
        }
    };
}
