/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import type { ColorTheme } from '../color.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ThemeDataContext } from '../../mol-theme/theme.js';
export declare const ChainIdColorThemeParams: {
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
    asymId: PD.Select<AsymIdType>;
};
export type ChainIdColorThemeParams = typeof ChainIdColorThemeParams;
export declare function getChainIdColorThemeParams(ctx: ThemeDataContext): {
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
    asymId: PD.Select<AsymIdType>;
};
type AsymIdType = 'auth' | 'label';
export declare function ChainIdColorTheme(ctx: ThemeDataContext, props: PD.Values<ChainIdColorThemeParams>): ColorTheme<ChainIdColorThemeParams>;
export declare const ChainIdColorThemeProvider: ColorTheme.Provider<ChainIdColorThemeParams, 'chain-id'>;
export {};
