/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { OrderedSet } from '../../mol-data/int/ordered-set.js';
import { Mat4 } from '../linear-algebra/3d/mat4.js';
import { Vec3 } from '../linear-algebra/3d/vec3.js';
import { Tensor } from '../linear-algebra/tensor.js';
import { Box3D } from './primitives/box3d.js';
export interface PositionData {
    x: ArrayLike<number>;
    y: ArrayLike<number>;
    z: ArrayLike<number>;
    /** subset of indices into the x/y/z/radius arrays */
    indices: OrderedSet;
    /** optional element radius */
    radius?: ArrayLike<number>;
    /** optional element id */
    id?: ArrayLike<number>;
}
export type DensityData = {
    transform: Mat4;
    field: Tensor;
    idField: Tensor;
    resolution: number;
    maxRadius: number;
};
export interface RegularGrid3d {
    box: Box3D;
    dimensions: Vec3;
}
export declare function getRegularGrid3dDelta({ box, dimensions }: RegularGrid3d): Vec3;
export declare function fillGridDim(length: number, start: number, step: number): Float32Array<ArrayBuffer>;
