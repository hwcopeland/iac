/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sebastian Bittrich <sebastian.bittrich@rcsb.org>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
import { ThemeDataContext } from '../../../mol-theme/theme.js';
import { ColorTheme } from '../../../mol-theme/color.js';
export declare const AccessibleSurfaceAreaColorThemeParams: {
    list: PD.ColorList;
};
export type AccessibleSurfaceAreaColorThemeParams = typeof AccessibleSurfaceAreaColorThemeParams;
export declare function getAccessibleSurfaceAreaColorThemeParams(ctx: ThemeDataContext): {
    list: PD.ColorList;
};
export declare function AccessibleSurfaceAreaColorTheme(ctx: ThemeDataContext, props: PD.Values<AccessibleSurfaceAreaColorThemeParams>): ColorTheme<AccessibleSurfaceAreaColorThemeParams>;
export declare const AccessibleSurfaceAreaColorThemeProvider: ColorTheme.Provider<AccessibleSurfaceAreaColorThemeParams, 'accessible-surface-area'>;
