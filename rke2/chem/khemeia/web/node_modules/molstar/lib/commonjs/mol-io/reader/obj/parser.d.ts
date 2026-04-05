/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ReaderResult as Result } from '../result.js';
import { Task } from '../../../mol-task/index.js';
import { Mesh } from '../../../mol-geo/geometry/mesh/mesh.js';
export declare function parse(data: string): Task<Result<Mesh>>;
