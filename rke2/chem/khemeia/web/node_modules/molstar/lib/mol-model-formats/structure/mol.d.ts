/**
 * Copyright (c) 2019-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Panagiotis Tourlas <panagiot_tourlov@hotmail.com>
 */
import { MolFile } from '../../mol-io/reader/mol/parser.js';
import { RuntimeContext, Task } from '../../mol-task/index.js';
import { ModelFormat } from '../format.js';
import { Trajectory } from '../../mol-model/structure.js';
export declare function getMolModels(mol: MolFile, format: ModelFormat<any> | undefined, ctx: RuntimeContext): Promise<import("../../mol-model/structure.js").ArrayTrajectory>;
export { MolFormat };
type MolFormat = ModelFormat<MolFile>;
declare namespace MolFormat {
    function is(x?: ModelFormat): x is MolFormat;
    function create(mol: MolFile): MolFormat;
}
export declare function trajectoryFromMol(mol: MolFile): Task<Trajectory>;
