"use strict";
/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 *
 * Adapted from three.js, The MIT License, Copyright © 2010-2020 three.js authors
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.StereoCamera = exports.DefaultStereoCameraProps = exports.StereoCameraParams = void 0;
const linear_algebra_1 = require("../../mol-math/linear-algebra.js");
const param_definition_1 = require("../../mol-util/param-definition.js");
const camera_1 = require("../camera.js");
const util_1 = require("./util.js");
exports.StereoCameraParams = {
    eyeSeparation: param_definition_1.ParamDefinition.Numeric(0.062, { min: 0.02, max: 0.1, step: 0.001 }, { description: 'Distance between left and right camera.' }),
    focus: param_definition_1.ParamDefinition.Numeric(10, { min: 1, max: 20, step: 0.1 }, { description: 'Apparent object distance.' }),
};
exports.DefaultStereoCameraProps = param_definition_1.ParamDefinition.getDefaultValues(exports.StereoCameraParams);
class StereoCamera {
    get viewport() {
        return this.parent.viewport;
    }
    get viewOffset() {
        return this.parent.viewOffset;
    }
    constructor(parent, props = {}) {
        this.parent = parent;
        this.left = new EyeCamera();
        this.right = new EyeCamera();
        this.props = { ...exports.DefaultStereoCameraProps, ...props };
    }
    setProps(props) {
        Object.assign(this.props, props);
    }
    update(xr) {
        this.parent.update();
        if (xr) {
            xrUpdate(this.parent, this.left, this.right, xr);
        }
        else {
            update(this.parent, this.props, this.left, this.right);
        }
    }
}
exports.StereoCamera = StereoCamera;
(function (StereoCamera) {
    function is(camera) {
        return 'left' in camera && 'right' in camera;
    }
    StereoCamera.is = is;
})(StereoCamera || (exports.StereoCamera = StereoCamera = {}));
class EyeCamera {
    constructor() {
        this.viewport = util_1.Viewport.create(0, 0, 0, 0);
        this.view = (0, linear_algebra_1.Mat4)();
        this.projection = (0, linear_algebra_1.Mat4)();
        this.projectionView = (0, linear_algebra_1.Mat4)();
        this.inverseProjectionView = (0, linear_algebra_1.Mat4)();
        this.headRotation = (0, linear_algebra_1.Mat4)();
        this.viewEye = (0, linear_algebra_1.Mat4)();
        this.isAsymmetricProjection = true;
        this.state = camera_1.Camera.createDefaultSnapshot();
        this.viewOffset = camera_1.Camera.ViewOffset();
        this.far = 0;
        this.near = 0;
        this.fogFar = 0;
        this.fogNear = 0;
        this.forceFull = false;
        this.scale = 0;
        this.minTargetDistance = 0;
        this.disabled = false;
    }
    getRay(out, x, y) {
        linear_algebra_1.Mat4.getTranslation(out.origin, linear_algebra_1.Mat4.invert((0, linear_algebra_1.Mat4)(), this.view));
        linear_algebra_1.Vec3.set(out.direction, x, y, 0.5);
        (0, util_1.cameraUnproject)(out.direction, out.direction, this.viewport, this.inverseProjectionView);
        linear_algebra_1.Vec3.normalize(out.direction, linear_algebra_1.Vec3.sub(out.direction, out.direction, out.origin));
        return out;
    }
}
const tmpEyeLeft = linear_algebra_1.Mat4.identity();
const tmpEyeRight = linear_algebra_1.Mat4.identity();
function copyStates(parent, eye) {
    util_1.Viewport.copy(eye.viewport, parent.viewport);
    linear_algebra_1.Mat4.copy(eye.view, parent.view);
    linear_algebra_1.Mat4.copy(eye.projection, parent.projection);
    linear_algebra_1.Mat4.copy(eye.headRotation, parent.headRotation);
    camera_1.Camera.copySnapshot(eye.state, parent.state);
    camera_1.Camera.copyViewOffset(eye.viewOffset, parent.viewOffset);
    eye.far = parent.far;
    eye.near = parent.near;
    eye.fogFar = parent.fogFar;
    eye.fogNear = parent.fogNear;
    eye.forceFull = parent.forceFull;
    eye.scale = parent.scale;
    eye.minTargetDistance = parent.minTargetDistance;
}
//
function update(camera, props, left, right) {
    // Copy the states
    copyStates(camera, left);
    copyStates(camera, right);
    // update the view offsets
    const w = Math.floor(camera.viewport.width / 2);
    const aspect = w / camera.viewport.height;
    left.viewport.width = w;
    right.viewport.x += w;
    right.viewport.width -= w;
    // update the projection and view matrices
    const eyeSepHalf = props.eyeSeparation / 2;
    const eyeSepOnProjection = eyeSepHalf * camera.near / props.focus;
    const ymax = camera.near * Math.tan(camera.state.fov * 0.5);
    let xmin, xmax;
    // translate xOffset
    tmpEyeLeft[12] = -eyeSepHalf;
    tmpEyeRight[12] = eyeSepHalf;
    // for left eye
    xmin = -ymax * aspect + eyeSepOnProjection;
    xmax = ymax * aspect + eyeSepOnProjection;
    left.projection[0] = 2 * camera.near / (xmax - xmin);
    left.projection[8] = (xmax + xmin) / (xmax - xmin);
    linear_algebra_1.Mat4.mul(left.view, left.view, tmpEyeLeft);
    linear_algebra_1.Mat4.mul(left.projectionView, left.projection, left.view);
    linear_algebra_1.Mat4.invert(left.inverseProjectionView, left.projectionView);
    // for right eye
    xmin = -ymax * aspect - eyeSepOnProjection;
    xmax = ymax * aspect - eyeSepOnProjection;
    right.projection[0] = 2 * camera.near / (xmax - xmin);
    right.projection[8] = (xmax + xmin) / (xmax - xmin);
    linear_algebra_1.Mat4.mul(right.view, right.view, tmpEyeRight);
    linear_algebra_1.Mat4.mul(right.projectionView, right.projection, right.view);
    linear_algebra_1.Mat4.invert(right.inverseProjectionView, right.projectionView);
    // ensure enabled
    left.disabled = false;
    right.disabled = false;
}
//
function xrUpdate(camera, left, right, xr) {
    _xrUpdate(camera, left, xr.pose.views[0], xr.layer);
    if (xr.pose.views.length === 1) {
        right.disabled = true;
    }
    else {
        _xrUpdate(camera, right, xr.pose.views[1], xr.layer);
    }
}
function _xrUpdate(camera, eye, view, layer) {
    copyStates(camera, eye);
    const lvp = layer.getViewport(view);
    util_1.Viewport.set(eye.viewport, lvp.x, lvp.y, lvp.width, lvp.height);
    linear_algebra_1.Mat4.fromArray(eye.projection, view.projectionMatrix, 0);
    linear_algebra_1.Mat4.fromArray(eye.view, view.transform.inverse.matrix, 0);
    linear_algebra_1.Mat4.mul(eye.projectionView, eye.projection, eye.view);
    linear_algebra_1.Mat4.invert(eye.inverseProjectionView, eye.projectionView);
    eye.disabled = false;
}
