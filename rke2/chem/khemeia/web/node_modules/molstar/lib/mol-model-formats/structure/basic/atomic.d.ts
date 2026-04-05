/**
 * Copyright (c) 2017-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Column } from '../../../mol-data/db.js';
import { SymmetryOperator } from '../../../mol-math/geometry.js';
import { ChainIndex } from '../../../mol-model/structure.js';
import { Model } from '../../../mol-model/structure/model/model.js';
import { AtomicConformation, AtomicHierarchy } from '../../../mol-model/structure/model/properties/atomic.js';
import { Entities } from '../../../mol-model/structure/model/properties/common.js';
import { ModelFormat } from '../../format.js';
import { AtomSite } from './schema.js';
export declare function getAtomicHierarchyAndConformation(atom_site: AtomSite, sourceIndex: Column<number>, entities: Entities, chemicalComponentMap: Model['properties']['chemicalComponentMap'], format: ModelFormat, previous?: Model): {
    sameAsPrevious: boolean;
    hierarchy: AtomicHierarchy;
    conformation: AtomicConformation;
    chainOperatorMapping: Map<ChainIndex, SymmetryOperator>;
};
