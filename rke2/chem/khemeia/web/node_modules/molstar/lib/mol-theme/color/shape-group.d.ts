/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import type { ColorTheme } from '../color.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ThemeDataContext } from '../../mol-theme/theme.js';
export declare const ShapeGroupColorThemeParams: {};
export type ShapeGroupColorThemeParams = typeof ShapeGroupColorThemeParams;
export declare function getShapeGroupColorThemeParams(ctx: ThemeDataContext): {};
export declare function ShapeGroupColorTheme(ctx: ThemeDataContext, props: PD.Values<ShapeGroupColorThemeParams>): ColorTheme<ShapeGroupColorThemeParams>;
export declare const ShapeGroupColorThemeProvider: ColorTheme.Provider<ShapeGroupColorThemeParams, 'shape-group'>;
