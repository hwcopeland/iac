"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.RayHelper = void 0;
const ray3d_1 = require("../../mol-math/geometry/primitives/ray3d.js");
const linear_algebra_1 = require("../../mol-math/linear-algebra.js");
const misc_1 = require("../../mol-math/misc.js");
const debug_1 = require("../../mol-util/debug.js");
const camera_1 = require("../camera.js");
const util_1 = require("../camera/util.js");
const util_2 = require("../camera/util.js");
const pick_1 = require("../passes/pick.js");
const sphere3d_1 = require("../../mol-math/geometry/primitives/sphere3d.js");
class RayHelper {
    setPickPadding(pickPadding) {
        if (this.pickPadding !== pickPadding) {
            this.pickPadding = pickPadding;
            this.update();
        }
    }
    update() {
        const size = this.pickPadding * 2 + 1;
        util_2.Viewport.set(this.viewport, 0, 0, size, size);
        this.buffers.setViewport(0, 0, size, size);
        this.spiral = (0, misc_1.spiral2d)(this.pickPadding);
        this.size = size;
        this.pickPass.setSize(size, size);
    }
    render(camera) {
        if (debug_1.isTimingMode)
            this.webgl.timer.mark('RayHelper.render', { captureStats: true });
        const { renderer, scene, helper } = this;
        renderer.setTransparentBackground(false);
        renderer.setDrawingBufferSize(this.size, this.size);
        renderer.setPixelRatio(1);
        renderer.setViewport(0, 0, this.size, this.size);
        this.pickPass.render(renderer, camera, scene, helper);
        if (debug_1.isTimingMode)
            this.webgl.timer.markEnd('RayHelper.render');
    }
    identifyInternal(x, y) {
        if (this.webgl.isContextLost)
            return;
        const { viewport } = this;
        const pickingId = this.buffers.getPickingId(x, y);
        if (pickingId === undefined)
            return;
        const z = this.buffers.getDepth(x, y);
        const position = linear_algebra_1.Vec3.create(x, y, z);
        (0, util_1.cameraUnproject)(position, position, viewport, this.camera.inverseProjectionView);
        return { id: pickingId, position };
    }
    prepare(ray, cam) {
        this.camera.far = cam.far;
        this.camera.near = cam.near;
        this.camera.fogFar = cam.fogFar;
        this.camera.fogNear = cam.fogNear;
        this.camera.forceFull = cam.forceFull;
        this.camera.scale = cam.scale;
        util_2.Viewport.copy(this.camera.viewport, this.viewport);
        camera_1.Camera.copySnapshot(this.camera.state, { ...cam.state, mode: 'orthographic' });
        updateOrthoRayCamera(this.camera, ray, cam.up);
        linear_algebra_1.Mat4.mul(this.camera.projectionView, this.camera.projection, this.camera.view);
        linear_algebra_1.Mat4.tryInvert(this.camera.inverseProjectionView, this.camera.projectionView);
        linear_algebra_1.Mat4.copy(this.camera.viewEye, cam.view);
    }
    getPickData() {
        const c = this.pickPadding;
        for (const d of this.spiral) {
            const pickData = this.identifyInternal(c + d[0], c + d[1]);
            if (pickData)
                return pickData;
        }
    }
    intersectsScene(ray, scale) {
        sphere3d_1.Sphere3D.scaleNX(this.sphere, this.scene.boundingSphereVisible, scale);
        return ray3d_1.Ray3D.isInsideSphere3D(ray, this.sphere) || ray3d_1.Ray3D.isIntersectingSphere3D(ray, this.sphere);
    }
    identify(ray, cam) {
        if (!this.intersectsScene(ray, cam.scale))
            return;
        this.prepare(ray, cam);
        if (debug_1.isTimingMode)
            this.webgl.timer.mark('RayHelper.identify');
        this.render(this.camera);
        this.buffers.read();
        if (debug_1.isTimingMode)
            this.webgl.timer.markEnd('RayHelper.identify');
        return this.getPickData();
    }
    asyncIdentify(ray, cam) {
        if (!this.intersectsScene(ray, cam.scale))
            return;
        this.prepare(ray, cam);
        if (debug_1.isTimingMode)
            this.webgl.timer.mark('RayHelper.asyncIdentify');
        this.render(this.camera);
        this.buffers.asyncRead();
        if (debug_1.isTimingMode)
            this.webgl.timer.markEnd('RayHelper.asyncIdentify');
        return {
            tryGet: () => {
                const status = this.buffers.check();
                if (status === pick_1.AsyncPickStatus.Resolved) {
                    return this.getPickData();
                }
                else if (status === pick_1.AsyncPickStatus.Pending) {
                    return 'pending';
                }
            }
        };
    }
    reset() {
        this.buffers.reset();
        this.pickPass.reset();
    }
    dispose() {
        this.buffers.dispose();
        this.pickPass.dispose();
    }
    constructor(webgl, renderer, scene, helper, options) {
        this.webgl = webgl;
        this.renderer = renderer;
        this.scene = scene;
        this.helper = helper;
        this.viewport = (0, util_2.Viewport)();
        this.sphere = (0, sphere3d_1.Sphere3D)();
        const size = options.pickPadding * 2 + 1;
        this.camera = new camera_1.Camera();
        this.pickPass = new pick_1.PickPass(webgl, size, size, 1);
        this.buffers = new pick_1.PickBuffers(this.webgl, this.pickPass, options.maxAsyncReadLag);
        this.pickPadding = options.pickPadding;
        this.update();
        if (!(0, pick_1.checkAsyncPickingSupport)(webgl)) {
            this.asyncIdentify = (ray, cam) => ({
                tryGet: () => this.identify(ray, cam)
            });
        }
    }
}
exports.RayHelper = RayHelper;
//
function updateOrthoRayCamera(camera, ray, up) {
    const { near, far, viewport } = camera;
    const height = 2 * Math.tan((0, misc_1.degToRad)(0.1) / 2) * linear_algebra_1.Vec3.distance(camera.position, camera.target) * camera.scale;
    const zoom = viewport.height / height;
    const fullLeft = -viewport.width / 2;
    const fullRight = viewport.width / 2;
    const fullTop = viewport.height / 2;
    const fullBottom = -viewport.height / 2;
    const dx = (fullRight - fullLeft) / (2 * zoom);
    const dy = (fullTop - fullBottom) / (2 * zoom);
    const cx = (fullRight + fullLeft) / 2;
    const cy = (fullTop + fullBottom) / 2;
    const left = cx - dx;
    const right = cx + dx;
    const top = cy + dy;
    const bottom = cy - dy;
    // build projection matrix
    linear_algebra_1.Mat4.ortho(camera.projection, left, right, top, bottom, near, far);
    const direction = linear_algebra_1.Vec3.normalize((0, linear_algebra_1.Vec3)(), ray.direction);
    const r = linear_algebra_1.Quat.fromUnitVec3((0, linear_algebra_1.Quat)(), direction, linear_algebra_1.Vec3.negUnitZ);
    linear_algebra_1.Quat.invert(r, r);
    const eye = linear_algebra_1.Vec3.clone(ray.origin);
    const target = linear_algebra_1.Vec3.add((0, linear_algebra_1.Vec3)(), eye, direction);
    // build view matrix
    linear_algebra_1.Mat4.lookAt(camera.view, eye, target, up);
}
