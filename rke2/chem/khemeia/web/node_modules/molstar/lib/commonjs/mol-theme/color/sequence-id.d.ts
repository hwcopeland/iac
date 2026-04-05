/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import type { ColorTheme } from '../color.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ThemeDataContext } from '../../mol-theme/theme.js';
export declare const SequenceIdColorThemeParams: {
    list: PD.ColorList;
};
export type SequenceIdColorThemeParams = typeof SequenceIdColorThemeParams;
export declare function getSequenceIdColorThemeParams(ctx: ThemeDataContext): {
    list: PD.ColorList;
};
export declare function SequenceIdColorTheme(ctx: ThemeDataContext, props: PD.Values<SequenceIdColorThemeParams>): ColorTheme<SequenceIdColorThemeParams>;
export declare const SequenceIdColorThemeProvider: ColorTheme.Provider<SequenceIdColorThemeParams, 'sequence-id'>;
