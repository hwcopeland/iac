/**
 * Copyright (c) 2017 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import * as Data from '../data-model.js';
import { ReaderResult as Result } from '../../result.js';
import { Task } from '../../../../mol-task/index.js';
export declare function parseCifBinary(data: Uint8Array): Task<Result<Data.CifFile>>;
