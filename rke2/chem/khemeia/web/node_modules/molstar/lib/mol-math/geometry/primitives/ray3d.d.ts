/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Gianluca Tomasello <giagitom@gmail.com>
 */
import { Mat4 } from '../../linear-algebra/3d/mat4.js';
import { Vec3 } from '../../linear-algebra/3d/vec3.js';
import { Box3D } from './box3d.js';
import { Sphere3D } from './sphere3d.js';
interface Ray3D {
    origin: Vec3;
    direction: Vec3;
}
declare function Ray3D(): Ray3D;
declare namespace Ray3D {
    function create(origin: Vec3, direction: Vec3): Ray3D;
    function copy(out: Ray3D, r: Ray3D): Ray3D;
    function clone(r: Ray3D): Ray3D;
    function targetTo(out: Ray3D, ray: Ray3D, target: Vec3): Ray3D;
    /** Transform ray with a Mat4 */
    function transform(out: Ray3D, ray: Ray3D, m: Mat4): Ray3D;
    function intersectSphere3D(out: Vec3, ray: Ray3D, sphere: Sphere3D): boolean;
    function isIntersectingSphere3D(ray: Ray3D, sphere: Sphere3D): boolean;
    function isInsideSphere3D(ray: Ray3D, sphere: Sphere3D): boolean;
    function intersectBox3D(out: Vec3, ray: Ray3D, box: Box3D): boolean;
    function isIntersectingBox3D(ray: Ray3D, box: Box3D): boolean;
}
export { Ray3D };
