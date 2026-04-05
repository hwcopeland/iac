"use strict";
/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.PickHelper = void 0;
const vec3_1 = require("../../mol-math/linear-algebra/3d/vec3.js");
const misc_1 = require("../../mol-math/misc.js");
const debug_1 = require("../../mol-util/debug.js");
const stereo_1 = require("../camera/stereo.js");
const util_1 = require("../camera/util.js");
const pick_1 = require("../passes/pick.js");
class PickHelper {
    setViewport(x, y, width, height) {
        util_1.Viewport.set(this.viewport, x, y, width, height);
        this.update();
    }
    setPickPadding(pickPadding) {
        if (this.pickPadding !== pickPadding) {
            this.pickPadding = pickPadding;
            this.update();
        }
    }
    update() {
        const { x, y, width, height } = this.viewport;
        this.pickRatio = this.pickPass.pickRatio;
        this.pickX = Math.ceil(x * this.pickRatio);
        this.pickY = Math.ceil(y * this.pickRatio);
        const pickWidth = Math.floor(width * this.pickRatio);
        const pickHeight = Math.floor(height * this.pickRatio);
        if (pickWidth !== this.pickWidth || pickHeight !== this.pickHeight) {
            this.pickWidth = pickWidth;
            this.pickHeight = pickHeight;
            this.halfPickWidth = Math.floor(this.pickWidth / 2);
            this.buffers.setViewport(this.pickX, this.pickY, this.pickWidth, this.pickHeight);
        }
        this.spiral = (0, misc_1.spiral2d)(Math.ceil(this.pickRatio * this.pickPadding));
        this.dirty = true;
    }
    render(camera) {
        if (debug_1.isTimingMode)
            this.webgl.timer.mark('PickHelper.render', { captureStats: true });
        const { pickX, pickY, pickWidth, pickHeight, halfPickWidth } = this;
        const { renderer, scene, helper } = this;
        renderer.setTransparentBackground(false);
        renderer.setDrawingBufferSize(pickWidth, pickHeight);
        renderer.setPixelRatio(this.pickRatio);
        if (stereo_1.StereoCamera.is(camera)) {
            renderer.setViewport(pickX, pickY, halfPickWidth, pickHeight);
            this.pickPass.render(renderer, camera.left, scene, helper);
            renderer.setViewport(pickX + halfPickWidth, pickY, pickWidth - halfPickWidth, pickHeight);
            this.pickPass.render(renderer, camera.right, scene, helper);
        }
        else {
            renderer.setViewport(pickX, pickY, pickWidth, pickHeight);
            this.pickPass.render(renderer, camera, scene, helper);
        }
        this.dirty = false;
        if (debug_1.isTimingMode)
            this.webgl.timer.markEnd('PickHelper.render');
    }
    identifyInternal(x, y, camera) {
        if (this.webgl.isContextLost)
            return;
        const { webgl, pickRatio } = this;
        if (webgl.isContextLost)
            return;
        x *= webgl.pixelRatio;
        y *= webgl.pixelRatio;
        y = this.pickPass.drawingBufferHeight - y; // flip y
        const { viewport } = this;
        // check if within viewport
        if (x < viewport.x ||
            y < viewport.y ||
            x > viewport.x + viewport.width ||
            y > viewport.y + viewport.height)
            return;
        const xv = x - viewport.x;
        const yv = y - viewport.y;
        const xp = Math.floor(xv * pickRatio);
        const yp = Math.floor(yv * pickRatio);
        const pickingId = this.buffers.getPickingId(xp, yp);
        if (pickingId === undefined)
            return;
        const z = this.buffers.getDepth(xp, yp);
        const position = vec3_1.Vec3.create(x, y, z);
        if (stereo_1.StereoCamera.is(camera)) {
            const halfWidth = Math.floor(viewport.width / 2);
            if (x > viewport.x + halfWidth) {
                position[0] = viewport.x + (xv - halfWidth) * 2;
                (0, util_1.cameraUnproject)(position, position, viewport, camera.right.inverseProjectionView);
            }
            else {
                position[0] = viewport.x + xv * 2;
                (0, util_1.cameraUnproject)(position, position, viewport, camera.left.inverseProjectionView);
            }
        }
        else {
            (0, util_1.cameraUnproject)(position, position, viewport, camera.inverseProjectionView);
        }
        return { id: pickingId, position };
    }
    prepare() {
        if (this.pickRatio !== this.pickPass.pickRatio) {
            this.update();
        }
    }
    getPickData(x, y, camera) {
        for (const d of this.spiral) {
            const pickData = this.identifyInternal(x + d[0], y + d[1], camera);
            if (pickData)
                return pickData;
        }
    }
    identify(x, y, camera) {
        this.prepare();
        if (this.dirty) {
            if (debug_1.isTimingMode)
                this.webgl.timer.mark('PickHelper.identify');
            this.webgl.resources.finalizePrograms(['pick'], true);
            this.render(camera);
            this.buffers.read();
            if (debug_1.isTimingMode)
                this.webgl.timer.markEnd('PickHelper.identify');
        }
        return this.getPickData(x, y, camera);
    }
    asyncIdentify(x, y, camera) {
        this.prepare();
        if (this.dirty) {
            if (debug_1.isTimingMode)
                this.webgl.timer.mark('PickHelper.asyncIdentify');
            this.render(camera);
            this.buffers.asyncRead();
            if (debug_1.isTimingMode)
                this.webgl.timer.markEnd('PickHelper.asyncIdentify');
        }
        return {
            tryGet: () => {
                const status = this.buffers.check();
                if (status === pick_1.AsyncPickStatus.Resolved) {
                    return this.getPickData(x, y, camera);
                }
                else if (status === pick_1.AsyncPickStatus.Pending) {
                    return 'pending';
                }
                else if (status === pick_1.AsyncPickStatus.Failed) {
                    this.dirty = true;
                }
            }
        };
    }
    reset() {
        this.buffers.reset();
        this.dirty = true;
    }
    dispose() {
        this.buffers.dispose();
    }
    constructor(webgl, renderer, scene, helper, pickPass, viewport, options) {
        this.webgl = webgl;
        this.renderer = renderer;
        this.scene = scene;
        this.helper = helper;
        this.pickPass = pickPass;
        this.dirty = true;
        this.buffers = new pick_1.PickBuffers(this.webgl, this.pickPass);
        this.viewport = (0, util_1.Viewport)();
        this.setViewport(viewport.x, viewport.y, viewport.width, viewport.height);
        this.pickPadding = options.pickPadding;
        if (!(0, pick_1.checkAsyncPickingSupport)(webgl)) {
            this.asyncIdentify = (x, y, camera) => ({
                tryGet: () => this.identify(x, y, camera)
            });
        }
    }
}
exports.PickHelper = PickHelper;
