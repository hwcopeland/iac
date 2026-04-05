/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Task } from '../../mol-task/index.js';
import { XtcFile } from '../../mol-io/reader/xtc/parser.js';
import { Coordinates } from '../../mol-model/structure/coordinates.js';
export declare function coordinatesFromXtc(file: XtcFile): Task<Coordinates>;
