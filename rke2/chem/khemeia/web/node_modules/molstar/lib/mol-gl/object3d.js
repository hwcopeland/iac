/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Mat4 } from '../mol-math/linear-algebra/3d/mat4.js';
import { Vec3 } from '../mol-math/linear-algebra/3d/vec3.js';
export var Object3D;
(function (Object3D) {
    function create() {
        return {
            view: Mat4.identity(),
            position: Vec3.create(0, 0, 0),
            direction: Vec3.create(0, 0, -1),
            up: Vec3.create(0, 1, 0),
        };
    }
    Object3D.create = create;
})(Object3D || (Object3D = {}));
