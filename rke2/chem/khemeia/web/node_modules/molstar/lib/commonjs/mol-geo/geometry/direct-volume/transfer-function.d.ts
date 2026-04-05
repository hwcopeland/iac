/**
 * Copyright (c) 2018-2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { TextureImage } from '../../../mol-gl/renderable/util.js';
import { ValueCell } from '../../../mol-util/index.js';
import { Vec2 } from '../../../mol-math/linear-algebra.js';
export interface ControlPoint {
    x: number;
    alpha: number;
}
export declare function getControlPointsFromString(s: string): ControlPoint[];
export declare function getControlPointsFromVec2Array(array: Vec2[]): ControlPoint[];
export declare function createTransferFunctionTexture(controlPoints: ControlPoint[], texture?: ValueCell<TextureImage<Uint8Array>>): ValueCell<TextureImage<Uint8Array>>;
