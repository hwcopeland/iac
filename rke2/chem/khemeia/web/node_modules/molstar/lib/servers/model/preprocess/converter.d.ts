/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { CifFrame } from '../../../mol-io/reader/cif.js';
import { CifWriter } from '../../../mol-io/writer/cif.js';
export declare function classifyCif(frame: CifFrame): Promise<CifWriter.Category<any>[]>;
