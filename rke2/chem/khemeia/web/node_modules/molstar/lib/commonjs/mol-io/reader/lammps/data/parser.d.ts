/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Ludovic Autin <ludovic.autin@gmail.com>
 */
import { Task } from '../../../../mol-task/index.js';
import { ReaderResult as Result } from '../../result.js';
import { LammpsDataFile } from '../schema.js';
import { StringLike } from '../../../common/string-like.js';
export declare function parseLammpsData(data: StringLike): Task<Result<LammpsDataFile>>;
