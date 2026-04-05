/**
 * Copyright (c) 2017-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Zepei Xu <xuzepei19950617@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Eric E <etongfu@@outlook.com>
 */
import { ReaderResult as Result } from '../result.js';
import { Task } from '../../../mol-task/index.js';
import { StringLike } from '../../common/string-like.js';
import { Mol2File } from './schema.js';
export declare function parseMol2(data: StringLike, name: string): Task<Result<Mol2File>>;
