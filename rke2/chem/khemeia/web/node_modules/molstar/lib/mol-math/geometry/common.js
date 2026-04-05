/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Vec3 } from '../linear-algebra/3d/vec3.js';
import { Box3D } from './primitives/box3d.js';
export function getRegularGrid3dDelta({ box, dimensions }) {
    return Vec3.div(Vec3(), Box3D.size(Vec3(), box), Vec3.subScalar(Vec3(), dimensions, 1));
}
export function fillGridDim(length, start, step) {
    const a = new Float32Array(length);
    for (let i = 0; i < a.length; i++) {
        a[i] = start + (step * i);
    }
    return a;
}
