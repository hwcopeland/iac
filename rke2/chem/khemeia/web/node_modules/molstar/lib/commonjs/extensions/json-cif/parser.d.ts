/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { CifFile } from '../../mol-io/reader/cif.js';
import { ReaderResult } from '../../mol-io/reader/result.js';
import { Task } from '../../mol-task/index.js';
import { JSONCifFile } from './model.js';
export declare function parseJSONCif(data: JSONCifFile): CifFile;
export declare function parseJSONCifString(data: string): Task<ReaderResult<CifFile>>;
