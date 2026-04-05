"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ShaderManager = void 0;
const bloom_1 = require("../passes/bloom.js");
const illumination_1 = require("../passes/illumination.js");
const marking_1 = require("../passes/marking.js");
const postprocessing_1 = require("../passes/postprocessing.js");
class ShaderManager {
    static ensureRequired(webgl, scene, p) {
        const sm = new ShaderManager(webgl, scene);
        sm.updateRequired(p);
        sm.finalizeRequired(true);
    }
    constructor(webgl, scene) {
        this.webgl = webgl;
        this.scene = scene;
        this.required = [];
    }
    updateRequired(p) {
        this.required.length = 0;
        this.required.push('color');
        if (illumination_1.IlluminationPass.isEnabled(this.webgl, p.illumination)) {
            this.required.push('tracing');
        }
        if (marking_1.MarkingPass.isEnabled(p.marking) && this.scene.markerAverage > 0) {
            this.required.push('marking');
        }
        if (bloom_1.BloomPass.isEnabled(p.postprocessing) && this.scene.emissiveAverage > 0) {
            this.required.push('emissive');
        }
        if (postprocessing_1.PostprocessingPass.isTransparentDepthRequired(this.scene, p.postprocessing) || !this.webgl.extensions.drawBuffers || !this.webgl.extensions.depthTexture || illumination_1.IlluminationPass.isEnabled(this.webgl, p.illumination)) {
            this.required.push('depth');
        }
        this.webgl.resources.linkPrograms(this.required);
    }
    finalizeRequired(isSynchronous) {
        return this.finalize(this.required, isSynchronous);
    }
    finalize(variants, isSynchronous) {
        return this.webgl.resources.finalizePrograms(variants, isSynchronous);
    }
}
exports.ShaderManager = ShaderManager;
