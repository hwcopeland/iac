/**
 * Copyright (c) 2017-2025 Mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { IntraUnitBonds } from './data.js';
import { Unit } from '../../unit.js';
import { BondComputationProps } from './common.js';
declare function computeIntraUnitBonds(unit: Unit.Atomic, props?: Partial<BondComputationProps>): IntraUnitBonds;
export { computeIntraUnitBonds };
