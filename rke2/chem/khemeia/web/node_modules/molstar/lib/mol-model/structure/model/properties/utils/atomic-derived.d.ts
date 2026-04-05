/**
 * Copyright (c) 2018-2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { AtomicData } from '../atomic.js';
import { AtomicIndex, AtomicDerivedData, AtomicSegments } from '../atomic/hierarchy.js';
import { ChemicalComponentMap } from '../common.js';
export declare function getAtomicDerivedData(data: AtomicData, segments: AtomicSegments, index: AtomicIndex, chemicalComponentMap: ChemicalComponentMap): AtomicDerivedData;
