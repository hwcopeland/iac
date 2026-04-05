/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Unit, ElementIndex } from '../../mol-model/structure.js';
import type { ColorTheme } from '../color.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ThemeDataContext } from '../theme.js';
export declare const OccupancyColorThemeParams: {
    domain: PD.Interval;
    list: PD.ColorList;
};
export type OccupancyColorThemeParams = typeof OccupancyColorThemeParams;
export declare function getOccupancyColorThemeParams(ctx: ThemeDataContext): {
    domain: PD.Interval;
    list: PD.ColorList;
};
export declare function getOccupancy(unit: Unit, element: ElementIndex): number;
export declare function OccupancyColorTheme(ctx: ThemeDataContext, props: PD.Values<OccupancyColorThemeParams>): ColorTheme<OccupancyColorThemeParams>;
export declare const OccupancyColorThemeProvider: ColorTheme.Provider<OccupancyColorThemeParams, 'occupancy'>;
