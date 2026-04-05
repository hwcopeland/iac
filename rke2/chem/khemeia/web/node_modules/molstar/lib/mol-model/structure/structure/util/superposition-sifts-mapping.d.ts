/**
 * Copyright (c) 2021-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Sebastian Bittrich <sebastian.bittrich@rcsb.org>
 */
import { MinimizeRmsd } from '../../../../mol-math/linear-algebra/3d/minimize-rmsd.js';
import { ElementIndex, ResidueIndex } from '../../model/indexing.js';
import { StructureElement } from '../element.js';
import { Structure } from '../structure.js';
import { Unit } from '../unit.js';
export interface AlignmentResultEntry {
    transform: MinimizeRmsd.Result;
    pivot: number;
    other: number;
}
export interface AlignmentResult {
    entries: AlignmentResultEntry[];
    zeroOverlapPairs: [number, number][];
    failedPairs: [number, number][];
}
type IncludeResidueTest = (traceElementOrFirstAtom: StructureElement.Location<Unit.Atomic>, residueIndex: ResidueIndex, startIndex: ElementIndex, endIndex: ElementIndex) => boolean;
export declare function alignAndSuperposeWithSIFTSMapping(structures: Structure[], options?: {
    traceOnly?: boolean;
    includeResidueTest?: IncludeResidueTest;
}): AlignmentResult;
export {};
