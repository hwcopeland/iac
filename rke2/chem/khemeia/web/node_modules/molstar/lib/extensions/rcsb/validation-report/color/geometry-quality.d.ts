/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ThemeDataContext } from '../../../../mol-theme/theme.js';
import { ColorTheme } from '../../../../mol-theme/color.js';
import { ParamDefinition as PD } from '../../../../mol-util/param-definition.js';
import { ValidationReport } from '../prop.js';
export declare function getGeometricQualityColorThemeParams(ctx: ThemeDataContext): {
    ignore: PD.MultiSelect<string>;
};
export type GeometricQualityColorThemeParams = ReturnType<typeof getGeometricQualityColorThemeParams>;
export declare function GeometryQualityColorTheme(ctx: ThemeDataContext, props: PD.Values<GeometricQualityColorThemeParams>): ColorTheme<GeometricQualityColorThemeParams>;
export declare const GeometryQualityColorThemeProvider: ColorTheme.Provider<GeometricQualityColorThemeParams, ValidationReport.Tag.GeometryQuality>;
