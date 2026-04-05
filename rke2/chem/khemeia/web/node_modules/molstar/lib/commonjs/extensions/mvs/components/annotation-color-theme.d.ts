/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { SortedArray } from '../../../mol-data/int.js';
import type { ColorTheme } from '../../../mol-theme/color.js';
import type { ThemeDataContext } from '../../../mol-theme/theme.js';
import { Color } from '../../../mol-util/color/index.js';
import { ParamDefinition as PD } from '../../../mol-util/param-definition.js';
export declare const MVSCategoricalPaletteParams: {
    colors: PD.Mapped<PD.NamedParams<{
        kind: "interpolate" | "set";
        colors: import("../../../mol-util/color/color.js").ColorListEntry[];
    }, "list"> | PD.NamedParams<PD.Normalize<{
        value: string;
        color: Color;
    }>[], "dictionary">>;
    repeatColorList: PD.BooleanParam;
    sort: PD.Select<"none" | "numeric" | "lexical">;
    sortDirection: PD.Select<"ascending" | "descending">;
    caseInsensitive: PD.BooleanParam;
    setMissingColor: PD.BooleanParam;
    missingColor: PD.Color;
};
export type MVSCategoricalPaletteParams = typeof MVSCategoricalPaletteParams;
export type MVSCategoricalPaletteProps = PD.Values<MVSCategoricalPaletteParams>;
export declare const MVSDiscretePaletteParams: {
    colors: PD.ObjectList<PD.Normalize<{
        color: Color;
        fromValue: number;
        toValue: number;
    }>>;
    mode: PD.Select<"absolute" | "normalized">;
    xMin: PD.Converted<number | null, string>;
    xMax: PD.Converted<number | null, string>;
};
export type MVSDiscretePaletteParams = typeof MVSDiscretePaletteParams;
export type MVSDiscretePaletteProps = PD.Values<MVSDiscretePaletteParams>;
export declare const MVSContinuousPaletteParams: {
    colors: PD.ColorList;
    mode: PD.Select<"absolute" | "normalized">;
    xMin: PD.Converted<number | null, string>;
    xMax: PD.Converted<number | null, string>;
    setUnderflowColor: PD.BooleanParam;
    underflowColor: PD.Color;
    setOverflowColor: PD.BooleanParam;
    overflowColor: PD.Color;
};
export type MVSContinuousPaletteParams = typeof MVSContinuousPaletteParams;
export type MVSContinuousPaletteProps = PD.Values<MVSContinuousPaletteParams>;
/** Parameter definition for color theme "MVS Annotation" */
export declare const MVSAnnotationColorThemeParams: {
    annotationId: PD.Text<string>;
    fieldName: PD.Text<string>;
    background: PD.Color;
    palette: PD.Mapped<PD.NamedParams<PD.Normalize<{
        colors: {
            kind: "interpolate" | "set";
            colors: import("../../../mol-util/color/color.js").ColorListEntry[];
        };
        mode: "absolute" | "normalized";
        xMin: number | null;
        xMax: number | null;
        setUnderflowColor: boolean;
        underflowColor: Color;
        setOverflowColor: boolean;
        overflowColor: Color;
    }>, "continuous"> | PD.NamedParams<PD.Normalize<{
        colors: PD.Normalize<{
            color: /*elided*/ any;
            fromValue: /*elided*/ any;
            toValue: /*elided*/ any;
        }>[];
        mode: "absolute" | "normalized";
        xMin: number | null;
        xMax: number | null;
    }>, "discrete"> | PD.NamedParams<PD.Normalize<unknown>, "direct"> | PD.NamedParams<PD.Normalize<{
        colors: PD.NamedParams<{
            kind: "interpolate" | "set";
            colors: import("../../../mol-util/color/color.js").ColorListEntry[];
        }, "list"> | PD.NamedParams<PD.Normalize<{
            value: /*elided*/ any;
            color: /*elided*/ any;
        }>[], "dictionary">;
        repeatColorList: boolean;
        sort: "none" | "numeric" | "lexical";
        sortDirection: "ascending" | "descending";
        caseInsensitive: boolean;
        setMissingColor: boolean;
        missingColor: Color;
    }>, "categorical">>;
};
export type MVSAnnotationColorThemeParams = typeof MVSAnnotationColorThemeParams;
/** Parameter values for color theme "MVS Annotation" */
export type MVSAnnotationColorThemeProps = PD.Values<MVSAnnotationColorThemeParams>;
/** Return color theme that assigns colors based on an annotation file.
 * The annotation file itself is handled by a custom model property (`MVSAnnotationsProvider`),
 * the color theme then just uses this property. */
export declare function MVSAnnotationColorTheme(ctx: ThemeDataContext, props: MVSAnnotationColorThemeProps): ColorTheme<MVSAnnotationColorThemeParams>;
/** A thingy that is needed to register color theme "MVS Annotation" */
export declare const MVSAnnotationColorThemeProvider: ColorTheme.Provider<MVSAnnotationColorThemeParams, 'mvs-annotation'>;
export declare function makeContinuousPaletteCheckpoints(props: MVSContinuousPaletteProps): {
    colors: Color[];
    checkpoints: SortedArray<number>;
};
