/**
 * Copyright (c) 2018-2022 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { RuntimeContext } from '../../../mol-task/index.js';
import { PositionData } from '../common.js';
import { GaussianDensityProps, GaussianDensityData } from '../gaussian-density.js';
import { Box3D } from '../primitives/box3d.js';
export declare function GaussianDensityCPU(ctx: RuntimeContext, position: PositionData, box: Box3D, radius: (index: number) => number, props: GaussianDensityProps): Promise<GaussianDensityData>;
