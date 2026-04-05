/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { SortedArray } from '../../../../../mol-data/int.js';
import { StructureElement } from '../../../structure.js';
import { Unit } from '../../unit.js';
export declare function computeRings(unit: Unit.Atomic): SortedArray<StructureElement.UnitIndex>[];
export declare function getFingerprint(elements: string[]): string;
type RingIndex = import('../rings.js').UnitRings.Index;
export declare function createIndex(rings: ArrayLike<SortedArray<StructureElement.UnitIndex>>, aromaticRings: ReadonlyArray<RingIndex>): {
    elementRingIndices: Map<StructureElement.UnitIndex, import("../rings.js").UnitRings.Index[]>;
    elementAromaticRingIndices: Map<StructureElement.UnitIndex, import("../rings.js").UnitRings.Index[]>;
    ringComponentIndex: import("../rings.js").UnitRings.ComponentIndex[];
    ringComponents: import("../rings.js").UnitRings.Index[][];
};
export {};
