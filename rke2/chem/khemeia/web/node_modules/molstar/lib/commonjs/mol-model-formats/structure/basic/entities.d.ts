/**
 * Copyright (c) 2017-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Entities } from '../../../mol-model/structure/model/properties/common.js';
import { Model } from '../../../mol-model/structure/model.js';
import { BasicData } from './schema.js';
export declare function getEntityData(data: BasicData): Entities;
export declare function getEntitiesWithPRD(data: BasicData, entities: Entities, structAsymMap: Model['properties']['structAsymMap']): Entities;
