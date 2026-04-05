/**
 * Copyright (c) 2019-2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { PrincipalAxes } from '../../../../mol-math/linear-algebra/matrix/principal-axes.js';
import { Unit } from '../unit.js';
export declare function toPositionsArray(unit: Unit): Float32Array<ArrayBuffer>;
export declare function getPrincipalAxes(unit: Unit): PrincipalAxes;
