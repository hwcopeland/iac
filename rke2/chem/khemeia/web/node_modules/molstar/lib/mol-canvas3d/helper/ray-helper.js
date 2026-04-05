/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Ray3D } from '../../mol-math/geometry/primitives/ray3d.js';
import { Mat4, Quat, Vec3 } from '../../mol-math/linear-algebra.js';
import { degToRad, spiral2d } from '../../mol-math/misc.js';
import { isTimingMode } from '../../mol-util/debug.js';
import { Camera } from '../camera.js';
import { cameraUnproject } from '../camera/util.js';
import { Viewport } from '../camera/util.js';
import { PickBuffers, PickPass, checkAsyncPickingSupport, AsyncPickStatus } from '../passes/pick.js';
import { Sphere3D } from '../../mol-math/geometry/primitives/sphere3d.js';
export class RayHelper {
    setPickPadding(pickPadding) {
        if (this.pickPadding !== pickPadding) {
            this.pickPadding = pickPadding;
            this.update();
        }
    }
    update() {
        const size = this.pickPadding * 2 + 1;
        Viewport.set(this.viewport, 0, 0, size, size);
        this.buffers.setViewport(0, 0, size, size);
        this.spiral = spiral2d(this.pickPadding);
        this.size = size;
        this.pickPass.setSize(size, size);
    }
    render(camera) {
        if (isTimingMode)
            this.webgl.timer.mark('RayHelper.render', { captureStats: true });
        const { renderer, scene, helper } = this;
        renderer.setTransparentBackground(false);
        renderer.setDrawingBufferSize(this.size, this.size);
        renderer.setPixelRatio(1);
        renderer.setViewport(0, 0, this.size, this.size);
        this.pickPass.render(renderer, camera, scene, helper);
        if (isTimingMode)
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
        const position = Vec3.create(x, y, z);
        cameraUnproject(position, position, viewport, this.camera.inverseProjectionView);
        return { id: pickingId, position };
    }
    prepare(ray, cam) {
        this.camera.far = cam.far;
        this.camera.near = cam.near;
        this.camera.fogFar = cam.fogFar;
        this.camera.fogNear = cam.fogNear;
        this.camera.forceFull = cam.forceFull;
        this.camera.scale = cam.scale;
        Viewport.copy(this.camera.viewport, this.viewport);
        Camera.copySnapshot(this.camera.state, { ...cam.state, mode: 'orthographic' });
        updateOrthoRayCamera(this.camera, ray, cam.up);
        Mat4.mul(this.camera.projectionView, this.camera.projection, this.camera.view);
        Mat4.tryInvert(this.camera.inverseProjectionView, this.camera.projectionView);
        Mat4.copy(this.camera.viewEye, cam.view);
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
        Sphere3D.scaleNX(this.sphere, this.scene.boundingSphereVisible, scale);
        return Ray3D.isInsideSphere3D(ray, this.sphere) || Ray3D.isIntersectingSphere3D(ray, this.sphere);
    }
    identify(ray, cam) {
        if (!this.intersectsScene(ray, cam.scale))
            return;
        this.prepare(ray, cam);
        if (isTimingMode)
            this.webgl.timer.mark('RayHelper.identify');
        this.render(this.camera);
        this.buffers.read();
        if (isTimingMode)
            this.webgl.timer.markEnd('RayHelper.identify');
        return this.getPickData();
    }
    asyncIdentify(ray, cam) {
        if (!this.intersectsScene(ray, cam.scale))
            return;
        this.prepare(ray, cam);
        if (isTimingMode)
            this.webgl.timer.mark('RayHelper.asyncIdentify');
        this.render(this.camera);
        this.buffers.asyncRead();
        if (isTimingMode)
            this.webgl.timer.markEnd('RayHelper.asyncIdentify');
        return {
            tryGet: () => {
                const status = this.buffers.check();
                if (status === AsyncPickStatus.Resolved) {
                    return this.getPickData();
                }
                else if (status === AsyncPickStatus.Pending) {
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
        this.viewport = Viewport();
        this.sphere = Sphere3D();
        const size = options.pickPadding * 2 + 1;
        this.camera = new Camera();
        this.pickPass = new PickPass(webgl, size, size, 1);
        this.buffers = new PickBuffers(this.webgl, this.pickPass, options.maxAsyncReadLag);
        this.pickPadding = options.pickPadding;
        this.update();
        if (!checkAsyncPickingSupport(webgl)) {
            this.asyncIdentify = (ray, cam) => ({
                tryGet: () => this.identify(ray, cam)
            });
        }
    }
}
//
function updateOrthoRayCamera(camera, ray, up) {
    const { near, far, viewport } = camera;
    const height = 2 * Math.tan(degToRad(0.1) / 2) * Vec3.distance(camera.position, camera.target) * camera.scale;
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
    Mat4.ortho(camera.projection, left, right, top, bottom, near, far);
    const direction = Vec3.normalize(Vec3(), ray.direction);
    const r = Quat.fromUnitVec3(Quat(), direction, Vec3.negUnitZ);
    Quat.invert(r, r);
    const eye = Vec3.clone(ray.origin);
    const target = Vec3.add(Vec3(), eye, direction);
    // build view matrix
    Mat4.lookAt(camera.view, eye, target, up);
}
