/**
 * Copyright (c) 2019-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Task } from '../../mol-task/index.js';
import { DcdFile } from '../../mol-io/reader/dcd/parser.js';
import { Coordinates } from '../../mol-model/structure/coordinates.js';
export declare function coordinatesFromDcd(dcdFile: DcdFile): Task<Coordinates>;
