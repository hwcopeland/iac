/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Ludovic Autin <ludovic.autin@gmail.com>
 */
import { Coordinates } from '../../mol-model/structure/coordinates.js';
import { LammpsTrajectoryFile, UnitStyle } from '../../mol-io/reader/lammps/schema.js';
import { Task } from '../../mol-task/index.js';
import { Trajectory } from '../../mol-model/structure.js';
import { ModelFormat } from '../format.js';
export declare function coordinatesFromLammpsTrajectory(file: LammpsTrajectoryFile, unitsStyle?: UnitStyle): Task<Coordinates>;
export { LammpsTrajectoryFormat };
type LammpsTrajectoryFormat = ModelFormat<LammpsTrajectoryFile>;
declare namespace LammpsTrajectoryFormat {
    function is(x?: ModelFormat): x is LammpsTrajectoryFormat;
    function create(mol: LammpsTrajectoryFile): LammpsTrajectoryFormat;
}
export declare function trajectoryFromLammpsTrajectory(mol: LammpsTrajectoryFile, unitsStyle?: UnitStyle): Task<Trajectory>;
