/**
 * Copyright (c) 2019-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { OrderedSet } from '../../mol-data/int.js';
import { Loci } from '../../mol-model/loci.js';
import { StructureElement } from '../../mol-model/structure.js';
import { SequenceWrapper, StructureUnit } from './wrapper.js';
export declare class HeteroSequenceWrapper extends SequenceWrapper<StructureUnit> {
    private readonly unitMap;
    private readonly sequence;
    private readonly sequenceIndices;
    private readonly residueIndices;
    private readonly seqToUnit;
    residueLabel(seqIdx: number): string;
    residueColor(seqIdx: number): import("../../mol-util/color/index.js").Color;
    residueClass(seqIdx: number): string;
    getSeqIndices(loci: Loci): OrderedSet;
    getLoci(seqIdx: number): StructureElement.Loci;
    constructor(data: StructureUnit);
}
