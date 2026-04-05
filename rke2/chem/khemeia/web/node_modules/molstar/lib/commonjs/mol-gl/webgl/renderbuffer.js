"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.getFormat = getFormat;
exports.getAttachment = getAttachment;
exports.createRenderbuffer = createRenderbuffer;
const id_factory_1 = require("../../mol-util/id-factory.js");
const compat_1 = require("./compat.js");
const framebuffer_1 = require("./framebuffer.js");
const debug_1 = require("../../mol-util/debug.js");
const getNextRenderbufferId = (0, id_factory_1.idFactory)();
function getFormat(gl, format) {
    switch (format) {
        case 'depth16': return gl.DEPTH_COMPONENT16;
        case 'stencil8': return gl.STENCIL_INDEX8;
        case 'rgba4': return gl.RGBA4;
        case 'depth-stencil': return gl.DEPTH_STENCIL;
        case 'depth24':
            if ((0, compat_1.isWebGL2)(gl))
                return gl.DEPTH_COMPONENT24;
            else
                throw new Error('WebGL2 needed for `depth24` renderbuffer format');
        case 'depth32f':
            if ((0, compat_1.isWebGL2)(gl))
                return gl.DEPTH_COMPONENT32F;
            else
                throw new Error('WebGL2 needed for `depth32f` renderbuffer format');
        case 'depth24-stencil8':
            if ((0, compat_1.isWebGL2)(gl))
                return gl.DEPTH24_STENCIL8;
            else
                throw new Error('WebGL2 needed for `depth24-stencil8` renderbuffer format');
        case 'depth32f-stencil8':
            if ((0, compat_1.isWebGL2)(gl))
                return gl.DEPTH32F_STENCIL8;
            else
                throw new Error('WebGL2 needed for `depth32f-stencil8` renderbuffer format');
    }
}
function getFormatSize(format) {
    switch (format) {
        case 'depth16': return 4;
        case 'stencil8': return 2;
        case 'rgba4': return 4;
        case 'depth-stencil': return 4;
        case 'depth24': return 3;
        case 'depth32f': return 4;
        case 'depth24-stencil8': return 4;
        case 'depth32f-stencil8': return 5;
    }
}
function getAttachment(gl, attachment) {
    switch (attachment) {
        case 'depth': return gl.DEPTH_ATTACHMENT;
        case 'stencil': return gl.STENCIL_ATTACHMENT;
        case 'depth-stencil': return gl.DEPTH_STENCIL_ATTACHMENT;
        case 'color0': return gl.COLOR_ATTACHMENT0;
    }
}
function getRenderbuffer(gl) {
    const renderbuffer = gl.createRenderbuffer();
    if (renderbuffer === null) {
        throw new Error('Could not create WebGL renderbuffer');
    }
    return renderbuffer;
}
function createRenderbuffer(gl, format, attachment, _width, _height) {
    let _renderbuffer = getRenderbuffer(gl);
    const bind = () => gl.bindRenderbuffer(gl.RENDERBUFFER, _renderbuffer);
    const _format = getFormat(gl, format);
    const _attachment = getAttachment(gl, attachment);
    function init() {
        bind();
        gl.renderbufferStorage(gl.RENDERBUFFER, _format, _width, _height);
    }
    init();
    let destroyed = false;
    return {
        id: getNextRenderbufferId(),
        getByteCount: () => getFormatSize(format) * _width * _height,
        bind,
        attachFramebuffer: (framebuffer) => {
            framebuffer.bind();
            bind();
            gl.framebufferRenderbuffer(gl.FRAMEBUFFER, _attachment, gl.RENDERBUFFER, _renderbuffer);
            if (debug_1.isDebugMode)
                (0, framebuffer_1.checkFramebufferStatus)(gl);
        },
        detachFramebuffer: (framebuffer) => {
            framebuffer.bind();
            bind();
            gl.framebufferRenderbuffer(gl.FRAMEBUFFER, _attachment, gl.RENDERBUFFER, null);
            if (debug_1.isDebugMode)
                (0, framebuffer_1.checkFramebufferStatus)(gl);
        },
        setSize: (width, height) => {
            _width = width;
            _height = height;
            init();
        },
        reset: () => {
            _renderbuffer = getRenderbuffer(gl);
            init();
        },
        destroy: () => {
            if (destroyed)
                return;
            gl.deleteRenderbuffer(_renderbuffer);
            destroyed = true;
        }
    };
}
