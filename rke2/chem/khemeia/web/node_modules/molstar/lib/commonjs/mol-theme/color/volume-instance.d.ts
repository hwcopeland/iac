/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import type { ColorTheme } from '../color.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ThemeDataContext } from '../../mol-theme/theme.js';
export declare const VolumeInstanceColorThemeParams: {
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
export type VolumeInstanceColorThemeParams = typeof VolumeInstanceColorThemeParams;
export declare function getVolumeInstanceColorThemeParams(ctx: ThemeDataContext): {
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
export declare function VolumeInstanceColorTheme(ctx: ThemeDataContext, props: PD.Values<VolumeInstanceColorThemeParams>): ColorTheme<VolumeInstanceColorThemeParams>;
export declare const VolumeInstanceColorThemeProvider: ColorTheme.Provider<VolumeInstanceColorThemeParams, 'volume-instance'>;
