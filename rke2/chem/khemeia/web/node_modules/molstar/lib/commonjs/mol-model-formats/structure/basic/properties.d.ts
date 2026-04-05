/**
 * Copyright (c) 2017-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Model } from '../../../mol-model/structure/model/model.js';
import { AtomicHierarchy } from '../../../mol-model/structure/model/properties/atomic.js';
import { SaccharideComponentMap } from '../../../mol-model/structure/structure/carbohydrates/constants.js';
import { BasicData } from './schema.js';
export declare function getMissingResidues(data: BasicData): Model['properties']['missingResidues'];
export declare function getChemicalComponentMap(data: BasicData): Model['properties']['chemicalComponentMap'];
export declare function getSaccharideComponentMap(data: BasicData): SaccharideComponentMap;
export declare function getStructAsymMap(atomic: AtomicHierarchy, data?: BasicData): Model['properties']['structAsymMap'];
