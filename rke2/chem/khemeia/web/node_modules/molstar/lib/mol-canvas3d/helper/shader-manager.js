/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { BloomPass } from '../passes/bloom.js';
import { IlluminationPass } from '../passes/illumination.js';
import { MarkingPass } from '../passes/marking.js';
import { PostprocessingPass } from '../passes/postprocessing.js';
export class ShaderManager {
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
        if (IlluminationPass.isEnabled(this.webgl, p.illumination)) {
            this.required.push('tracing');
        }
        if (MarkingPass.isEnabled(p.marking) && this.scene.markerAverage > 0) {
            this.required.push('marking');
        }
        if (BloomPass.isEnabled(p.postprocessing) && this.scene.emissiveAverage > 0) {
            this.required.push('emissive');
        }
        if (PostprocessingPass.isTransparentDepthRequired(this.scene, p.postprocessing) || !this.webgl.extensions.drawBuffers || !this.webgl.extensions.depthTexture || IlluminationPass.isEnabled(this.webgl, p.illumination)) {
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
