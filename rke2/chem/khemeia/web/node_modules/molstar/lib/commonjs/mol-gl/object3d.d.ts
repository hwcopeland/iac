/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Mat4 } from '../mol-math/linear-algebra/3d/mat4.js';
import { Vec3 } from '../mol-math/linear-algebra/3d/vec3.js';
export interface Object3D {
    readonly view: Mat4;
    readonly position: Vec3;
    readonly direction: Vec3;
    readonly up: Vec3;
}
export declare namespace Object3D {
    function create(): Object3D;
}
