/**
 * Copyright (c) 2017-2025 Mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Structure } from '../../structure.js';
import { Unit } from '../../unit.js';
import { BondComputationProps } from './common.js';
import { InterUnitBonds } from './data.js';
export interface InterBondComputationProps extends BondComputationProps {
    validUnit: (unit: Unit) => boolean;
    validUnitPair: (structure: Structure, unitA: Unit, unitB: Unit) => boolean;
    ignoreWater: boolean;
    ignoreIon: boolean;
}
declare function computeInterUnitBonds(structure: Structure, props?: Partial<InterBondComputationProps>): InterUnitBonds;
export { computeInterUnitBonds };
