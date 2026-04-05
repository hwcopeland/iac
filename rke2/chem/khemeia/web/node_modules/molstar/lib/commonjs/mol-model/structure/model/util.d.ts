/**
 * Copyright (c) 2019-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Vec3 } from '../../../mol-math/linear-algebra.js';
import { AtomicConformation } from './properties/atomic.js';
import { CoarseConformation } from './properties/coarse.js';
import { Model } from './model.js';
export declare function calcModelCenter(atomicConformation: AtomicConformation, coarseConformation?: CoarseConformation): Vec3;
export declare function getAsymIdCount(model: Model): {
    auth: number;
    label: number;
};
