/**
 * Copyright (c) 2017-2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { AtomicData, AtomicSegments } from '../atomic.js';
import { Entities } from '../common.js';
import { AtomicIndex } from '../atomic/hierarchy.js';
export declare function getAtomicIndex(data: AtomicData, entities: Entities, segments: AtomicSegments): AtomicIndex;
