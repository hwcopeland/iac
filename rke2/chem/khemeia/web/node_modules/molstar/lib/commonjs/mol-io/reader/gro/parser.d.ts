/**
 * Copyright (c) 2017 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import * as Schema from './schema';
import { ReaderResult as Result } from '../result.js';
import { Task } from '../../../mol-task/index.js';
import { StringLike } from '../../common/string-like.js';
export declare function parseGRO(data: StringLike): Task<Result<Schema.GroFile>>;
