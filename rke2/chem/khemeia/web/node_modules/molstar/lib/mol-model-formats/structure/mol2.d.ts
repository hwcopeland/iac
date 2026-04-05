/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Task } from '../../mol-task/index.js';
import { ModelFormat } from '../format.js';
import { Mol2File } from '../../mol-io/reader/mol2/schema.js';
import { Trajectory } from '../../mol-model/structure.js';
export { Mol2Format };
type Mol2Format = ModelFormat<Mol2File>;
declare namespace Mol2Format {
    function is(x?: ModelFormat): x is Mol2Format;
    function create(mol2: Mol2File): Mol2Format;
}
export declare function trajectoryFromMol2(mol2: Mol2File): Task<Trajectory>;
