/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Progress } from '../../../mol-task/index.js';
import { Job } from './jobs.js';
import { StructureWrapper } from './structure-wrapper.js';
export interface Stats {
    structure: StructureWrapper;
    queryTimeMs: number;
    encodeTimeMs: number;
    resultSize: number;
}
export declare function resolveJob(job: Job): Promise<void>;
export declare function abortingObserver(p: Progress): void;
