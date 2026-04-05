/**
 * Copyright (c) 2017-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { RuntimeContext } from '../../../mol-task/index.js';
import { ModelFormat } from '../../format.js';
import { BasicData } from './schema.js';
import { ArrayTrajectory } from '../../../mol-model/structure/trajectory.js';
export declare function createModels(data: BasicData, format: ModelFormat, ctx: RuntimeContext): Promise<ArrayTrajectory>;
