/**
 * Copyright (c) 2019-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { GridLookup3D } from '../../../../mol-math/geometry.js';
import { SortedArray } from '../../../../mol-data/int.js';
import { Unit } from '../../../../mol-model/structure/structure.js';
import { ResidueIndex } from '../../../../mol-model/structure.js';
export declare function calcUnitProteinTraceLookup3D(unit: Unit.Atomic, unitProteinResidues: SortedArray<ResidueIndex>): GridLookup3D;
