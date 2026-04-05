/**
 * Copyright (c) 2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Column } from '../../../mol-data/db.js';
import { Task } from '../../../mol-task/index.js';
import { StringLike } from '../../common/string-like.js';
import { ReaderResult as Result } from '../result.js';
export interface XyzFile {
    readonly molecules: {
        readonly comment: string;
        readonly count: number;
        readonly x: Column<number>;
        readonly y: Column<number>;
        readonly z: Column<number>;
        readonly type_symbol: Column<string>;
    }[];
}
export declare function parseXyz(data: StringLike): Task<Result<XyzFile>>;
