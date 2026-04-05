"use strict";
/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Passes = void 0;
const draw_1 = require("./draw.js");
const pick_1 = require("./pick.js");
const multi_sample_1 = require("./multi-sample.js");
const illumination_1 = require("./illumination.js");
class Passes {
    constructor(webgl, assetManager, attribs = {}) {
        this.webgl = webgl;
        const drs = this.webgl.getDrawingBufferSize();
        this.draw = new draw_1.DrawPass(webgl, assetManager, drs.width, drs.height, attribs.transparency || 'blended');
        this.pick = new pick_1.PickPass(webgl, drs.width, drs.height, attribs.pickScale || 0.25);
        this.multiSample = new multi_sample_1.MultiSamplePass(webgl, this.draw);
        this.illumination = new illumination_1.IlluminationPass(webgl, this.draw);
    }
    getByteCount() {
        return this.draw.getByteCount() + this.pick.getByteCount() + this.multiSample.getByteCount() + this.illumination.getByteCount();
    }
    setPickScale(pickScale) {
        this.pick.setPickScale(pickScale);
    }
    setTransparency(transparency) {
        this.draw.setTransparency(transparency);
    }
    updateSize() {
        const drs = this.webgl.getDrawingBufferSize();
        // Avoid setting dimensions to 0x0 because it causes "empty textures are not allowed" error.
        const width = Math.max(drs.width, 2);
        const height = Math.max(drs.height, 2);
        this.draw.setSize(width, height);
        this.pick.setSize(width, height);
        this.multiSample.syncSize();
        this.illumination.setSize(width, height);
    }
}
exports.Passes = Passes;
