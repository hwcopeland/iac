/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Gianluca Tomasello <giagitom@gmail.com>
 */
import { Lookup3D } from './common.js';
import { PositionData } from '../common.js';
import { Vec3 } from '../../linear-algebra.js';
import { Boundary } from '../boundary.js';
interface GridLookup3D<T = number> extends Lookup3D<T> {
    readonly buckets: {
        readonly offset: ArrayLike<number>;
        readonly count: ArrayLike<number>;
        readonly array: ArrayLike<number>;
    };
}
declare function GridLookup3D<T extends number = number>(data: PositionData, boundary: Boundary, cellSizeOrCount?: Vec3 | number): GridLookup3D<T>;
export { GridLookup3D };
