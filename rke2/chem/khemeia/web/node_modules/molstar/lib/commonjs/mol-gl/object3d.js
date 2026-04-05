"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Object3D = void 0;
const mat4_1 = require("../mol-math/linear-algebra/3d/mat4.js");
const vec3_1 = require("../mol-math/linear-algebra/3d/vec3.js");
var Object3D;
(function (Object3D) {
    function create() {
        return {
            view: mat4_1.Mat4.identity(),
            position: vec3_1.Vec3.create(0, 0, 0),
            direction: vec3_1.Vec3.create(0, 0, -1),
            up: vec3_1.Vec3.create(0, 1, 0),
        };
    }
    Object3D.create = create;
})(Object3D || (exports.Object3D = Object3D = {}));
