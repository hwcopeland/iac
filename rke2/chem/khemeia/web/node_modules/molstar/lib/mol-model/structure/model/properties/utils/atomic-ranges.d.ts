/**
 * Copyright (c) 2018-2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { AtomicRanges, AtomicHierarchy } from '../atomic/hierarchy.js';
import { AtomicConformation } from '../atomic/conformation.js';
import { Entities } from '../common.js';
import { StructureSequence } from '../sequence.js';
export declare function getAtomicRanges(hierarchy: AtomicHierarchy, entities: Entities, conformation: AtomicConformation, sequence: StructureSequence): AtomicRanges;
