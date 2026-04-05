/**
 * Copyright (c) 2019-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ReaderResult as Result } from '../result.js';
import { Task } from '../../../mol-task/index.js';
import { PlyFile } from './schema.js';
import { StringLike } from '../../common/string-like.js';
export declare function parsePly(data: StringLike): Task<Result<PlyFile>>;
