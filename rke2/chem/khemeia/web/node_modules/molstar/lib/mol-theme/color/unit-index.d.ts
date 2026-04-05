/**
 * Copyright (c) 2018-2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import type { ColorTheme } from '../color.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ThemeDataContext } from '../../mol-theme/theme.js';
export declare const UnitIndexColorThemeParams: {
    palette: PD.Mapped<PD.NamedParams<PD.Normalize<{
        maxCount: number;
        hue: [number, number];
        chroma: [number, number];
        luminance: [number, number];
        sort: "none" | "contrast";
        clusteringStepCount: number;
        minSampleCount: number;
        sampleCountFactor: number;
    }>, "generate"> | PD.NamedParams<PD.Normalize<{
        list: {
            kind: "interpolate" | "set";
            colors: import("../../mol-util/color/color.js").ColorListEntry[];
        };
    }>, "colors">>;
};
export type UnitIndexColorThemeParams = typeof UnitIndexColorThemeParams;
export declare function getUnitIndexColorThemeParams(ctx: ThemeDataContext): {
    palette: PD.Mapped<PD.NamedParams<PD.Normalize<{
        maxCount: number;
        hue: [number, number];
        chroma: [number, number];
        luminance: [number, number];
        sort: "none" | "contrast";
        clusteringStepCount: number;
        minSampleCount: number;
        sampleCountFactor: number;
    }>, "generate"> | PD.NamedParams<PD.Normalize<{
        list: {
            kind: "interpolate" | "set";
            colors: import("../../mol-util/color/color.js").ColorListEntry[];
        };
    }>, "colors">>;
};
export declare function UnitIndexColorTheme(ctx: ThemeDataContext, props: PD.Values<UnitIndexColorThemeParams>): ColorTheme<UnitIndexColorThemeParams>;
export declare const UnitIndexColorThemeProvider: ColorTheme.Provider<UnitIndexColorThemeParams, 'unit-index'>;
