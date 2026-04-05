"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MVSAnnotationColorThemeProvider = exports.MVSAnnotationColorThemeParams = exports.MVSContinuousPaletteParams = exports.MVSDiscretePaletteParams = exports.MVSCategoricalPaletteParams = void 0;
exports.MVSAnnotationColorTheme = MVSAnnotationColorTheme;
exports.makeContinuousPaletteCheckpoints = makeContinuousPaletteCheckpoints;
const int_1 = require("../../../mol-data/int.js");
const structure_1 = require("../../../mol-model/structure.js");
const color_1 = require("../../../mol-util/color/index.js");
const names_1 = require("../../../mol-util/color/names.js");
const param_definition_1 = require("../../../mol-util/param-definition.js");
const param_definition_2 = require("../helpers/param-definition.js");
const utils_1 = require("../helpers/utils.js");
const annotation_prop_1 = require("./annotation-prop.js");
const is_mvs_model_prop_1 = require("./is-mvs-model-prop.js");
exports.MVSCategoricalPaletteParams = {
    colors: param_definition_1.ParamDefinition.MappedStatic('list', {
        list: param_definition_1.ParamDefinition.ColorList('category-10', { description: 'List of colors.', presetKind: 'set' }),
        dictionary: param_definition_1.ParamDefinition.ObjectList({
            value: param_definition_1.ParamDefinition.Text(),
            color: param_definition_1.ParamDefinition.Color(names_1.ColorNames.white),
        }, e => `${e.value}: ${color_1.Color.toHexStyle(e.color)}`, { description: 'Mapping of annotation values to colors.' }),
    }),
    repeatColorList: param_definition_1.ParamDefinition.Boolean(false, { hideIf: g => g.colors.name !== 'list', description: 'Repeat color list once all colors are depleted (only applies if `colors` is a list).' }),
    sort: param_definition_1.ParamDefinition.Select('none', [['none', 'None'], ['lexical', 'Lexical'], ['numeric', 'Numeric']], { hideIf: g => g.colors.name !== 'list', description: 'Sort actual annotation values before assigning colors from a list (none = take values in order of their first occurrence).' }),
    sortDirection: param_definition_1.ParamDefinition.Select('ascending', [['ascending', 'Ascending'], ['descending', 'Descending']], { hideIf: g => g.colors.name !== 'list', description: 'Sort direction.' }),
    caseInsensitive: param_definition_1.ParamDefinition.Boolean(false, { description: 'Treat annotation values as case-insensitive strings.' }),
    setMissingColor: param_definition_1.ParamDefinition.Boolean(false, { description: 'Allow setting a color for missing values.' }),
    missingColor: param_definition_1.ParamDefinition.Color(names_1.ColorNames.white, { hideIf: g => !g.setMissingColor, description: 'Color to use when (a) `colors` is a dictionary and given key is not present, or (b) `color` is a list and there are more actual annotation values than listed colors and `repeat_color_list` is not true.' }),
};
exports.MVSDiscretePaletteParams = {
    colors: param_definition_1.ParamDefinition.ObjectList({
        color: param_definition_1.ParamDefinition.Color(names_1.ColorNames.white),
        fromValue: param_definition_1.ParamDefinition.Numeric(-Infinity),
        toValue: param_definition_1.ParamDefinition.Numeric(Infinity),
    }, e => `${color_1.Color.toHexStyle(e.color)} [${e.fromValue}, ${e.toValue}]`, { description: 'Mapping of annotation value ranges to colors.' }),
    mode: param_definition_1.ParamDefinition.Select('normalized', [['normalized', 'Normalized'], ['absolute', 'Absolute']], { description: 'Defines whether the annotation values should be normalized before assigning color based on checkpoints in `colors` (`x_normalized = (x - x_min) / (x_max - x_min)`, where `[x_min, x_max]` are either `value_domain` if provided, or the lowest and the highest value encountered in the annotation).' }),
    xMin: (0, param_definition_2.MaybeFloatParamDefinition)({ hideIf: g => g.mode !== 'normalized', placeholder: 'auto', description: 'Defines `x_min` for normalization of annotation values. If not provided, minimum of the actual values will be used. Only used when `mode` is `"normalized"' }),
    xMax: (0, param_definition_2.MaybeFloatParamDefinition)({ hideIf: g => g.mode !== 'normalized', placeholder: 'auto', description: 'Defines `x_max` for normalization of annotation values. If not provided, maximum of the actual values will be used. Only used when `mode` is `"normalized"' }),
};
exports.MVSContinuousPaletteParams = {
    colors: param_definition_1.ParamDefinition.ColorList('yellow-green', { description: 'List of colors, with optional checkpoints.', presetKind: 'scale', offsets: true }),
    mode: param_definition_1.ParamDefinition.Select('normalized', [['normalized', 'Normalized'], ['absolute', 'Absolute']], { description: 'Defines whether the annotation values should be normalized before assigning color based on checkpoints in `colors` (`x_normalized = (x - x_min) / (x_max - x_min)`, where `[x_min, x_max]` are either `value_domain` if provided, or the lowest and the highest value encountered in the annotation).' }),
    xMin: (0, param_definition_2.MaybeFloatParamDefinition)({ hideIf: g => g.mode !== 'normalized', placeholder: 'auto', description: 'Defines `x_min` for normalization of annotation values. If not provided, minimum of the actual values will be used. Only used when `mode` is `"normalized"' }),
    xMax: (0, param_definition_2.MaybeFloatParamDefinition)({ hideIf: g => g.mode !== 'normalized', placeholder: 'auto', description: 'Defines `x_max` for normalization of annotation values. If not provided, maximum of the actual values will be used. Only used when `mode` is `"normalized"' }),
    setUnderflowColor: param_definition_1.ParamDefinition.Boolean(false, { description: 'Allow setting a color for values below the lowest checkpoint.' }),
    underflowColor: param_definition_1.ParamDefinition.Color(names_1.ColorNames.white, { hideIf: g => !g.setUnderflowColor, description: 'Color for values below the lowest checkpoint.' }),
    setOverflowColor: param_definition_1.ParamDefinition.Boolean(false, { description: 'Allow setting a color for values above the highest checkpoint.' }),
    overflowColor: param_definition_1.ParamDefinition.Color(names_1.ColorNames.white, { hideIf: g => !g.setOverflowColor, description: 'Color for values above the highest checkpoint.' }),
};
/** Parameter definition for color theme "MVS Annotation" */
exports.MVSAnnotationColorThemeParams = {
    annotationId: param_definition_1.ParamDefinition.Text('', { description: 'Reference to "Annotation" custom model property' }),
    fieldName: param_definition_1.ParamDefinition.Text('color', { description: 'Annotation field (column) from which to take color values' }),
    background: param_definition_1.ParamDefinition.Color(names_1.ColorNames.gainsboro, { description: 'Color for elements without annotation' }),
    palette: param_definition_1.ParamDefinition.MappedStatic('direct', {
        'direct': param_definition_1.ParamDefinition.EmptyGroup(),
        'categorical': param_definition_1.ParamDefinition.Group(exports.MVSCategoricalPaletteParams),
        'discrete': param_definition_1.ParamDefinition.Group(exports.MVSDiscretePaletteParams),
        'continuous': param_definition_1.ParamDefinition.Group(exports.MVSContinuousPaletteParams),
    }),
};
/** Return color theme that assigns colors based on an annotation file.
 * The annotation file itself is handled by a custom model property (`MVSAnnotationsProvider`),
 * the color theme then just uses this property. */
function MVSAnnotationColorTheme(ctx, props) {
    let color = () => props.background;
    if (ctx.structure && !ctx.structure.isEmpty) {
        const { annotation } = (0, annotation_prop_1.getMVSAnnotationForStructure)(ctx.structure, props.annotationId);
        if (annotation) {
            const paletteFunction = makePaletteFunction(props.palette, annotation, props.fieldName);
            const colorForStructureElementLocation = (location) => {
                const annotValue = annotation === null || annotation === void 0 ? void 0 : annotation.getValueForLocation(location, props.fieldName);
                const color = annotValue !== undefined ? paletteFunction(annotValue) : undefined;
                return color !== null && color !== void 0 ? color : props.background;
            };
            const auxLocation = structure_1.StructureElement.Location.create(ctx.structure);
            color = (location) => {
                if (structure_1.StructureElement.Location.is(location)) {
                    return colorForStructureElementLocation(location);
                }
                else if (structure_1.Bond.isLocation(location)) {
                    // this will be applied for each bond twice, to get color of each half (a* refers to the adjacent atom, b* to the opposite atom)
                    auxLocation.unit = location.aUnit;
                    auxLocation.element = location.aUnit.elements[location.aIndex];
                    return colorForStructureElementLocation(auxLocation);
                }
                return props.background;
            };
        }
        else {
            console.error(`Annotation source "${props.annotationId}" not present`);
        }
    }
    return {
        factory: MVSAnnotationColorTheme,
        granularity: 'groupInstance',
        preferSmoothing: false,
        color: color,
        props: props,
        description: 'Assigns colors based on custom MolViewSpec annotation data.',
    };
}
/** A thingy that is needed to register color theme "MVS Annotation" */
exports.MVSAnnotationColorThemeProvider = {
    name: 'mvs-annotation',
    label: 'MVS Annotation',
    category: 'Miscellaneous', // ColorTheme.Category.Misc can cause webpack build error due to import ordering
    factory: MVSAnnotationColorTheme,
    getParams: ctx => exports.MVSAnnotationColorThemeParams,
    defaultValues: param_definition_1.ParamDefinition.getDefaultValues(exports.MVSAnnotationColorThemeParams),
    isApplicable: (ctx) => !!ctx.structure && (0, is_mvs_model_prop_1.isMVSStructure)(ctx.structure),
};
function makePaletteFunction(props, annotation, fieldName) {
    if (props.name === 'direct')
        return utils_1.decodeColor;
    if (props.name === 'categorical')
        return makePaletteFunctionCategorical(props.params, annotation, fieldName);
    if (props.name === 'discrete')
        return makePaletteFunctionDiscrete(props.params, annotation, fieldName);
    if (props.name === 'continuous')
        return makePaletteFunctionContinuous(props.params, annotation, fieldName);
    throw new Error(`NotImplementedError: makePaletteFunction for ${props.name}`);
}
function makePaletteFunctionCategorical(props, annotation, fieldName) {
    const colorMap = {};
    if (props.colors.name === 'dictionary') {
        for (const { value, color } of props.colors.params) {
            const key = props.caseInsensitive ? value.toUpperCase() : value;
            colorMap[key] = color;
        }
    }
    else if (props.colors.name === 'list') {
        const values = annotation.getDistinctValuesInField(fieldName, props.caseInsensitive);
        if (props.sort === 'lexical')
            values.sort();
        else if (props.sort === 'numeric')
            values.sort((a, b) => Number.parseFloat(a) - Number.parseFloat(b));
        if (props.sortDirection === 'descending')
            values.reverse();
        const colorList = props.colors.params.colors.map(color_1.Color.fromColorListEntry);
        let next = 0;
        for (const value of values) {
            colorMap[value] = colorList[next++];
            if (next >= colorList.length && props.repeatColorList)
                next = 0; // else will get index-out-of-range and assign undefined
        }
    }
    const missingColor = props.setMissingColor ? props.missingColor : undefined;
    if (props.caseInsensitive) {
        return (value) => { var _a; return (_a = colorMap[value.toUpperCase()]) !== null && _a !== void 0 ? _a : missingColor; };
    }
    else {
        return (value) => { var _a; return (_a = colorMap[value]) !== null && _a !== void 0 ? _a : missingColor; };
    }
}
function makePaletteFunctionDiscrete(props, annotation, fieldName) {
    if (props.colors.length === 0)
        return () => undefined;
    const scale = makeNumericPaletteScale(props, annotation, fieldName);
    return (value) => {
        const xAbs = parseFloat(value);
        if (isNaN(xAbs))
            return undefined;
        const x = scale(xAbs);
        for (let i = props.colors.length - 1; i >= 0; i--) {
            const { color, fromValue, toValue } = props.colors[i];
            if (fromValue <= x && x <= toValue)
                return color;
        }
    };
}
function makePaletteFunctionContinuous(props, annotation, fieldName) {
    const { colors, checkpoints } = makeContinuousPaletteCheckpoints(props);
    if (colors.length === 0)
        return () => undefined;
    const scale = makeNumericPaletteScale(props, annotation, fieldName);
    const underflowColor = props.setUnderflowColor ? props.underflowColor : undefined;
    const overflowColor = props.setOverflowColor ? props.overflowColor : undefined;
    return (value) => {
        const xAbs = parseFloat(value);
        if (isNaN(xAbs))
            return undefined;
        const x = scale(xAbs);
        const gteIdx = int_1.SortedArray.findPredecessorIndex(checkpoints, x); // Index of the first greater or equal checkpoint
        if (gteIdx === 0) {
            if (x === checkpoints[0])
                return colors[0];
            else
                return underflowColor;
        }
        if (gteIdx === checkpoints.length) {
            return overflowColor;
        }
        const q = (x - checkpoints[gteIdx - 1]) / (checkpoints[gteIdx] - checkpoints[gteIdx - 1]);
        return color_1.Color.interpolate(colors[gteIdx - 1], colors[gteIdx], q);
    };
}
function makeNumericPaletteScale(props, annotation, fieldName) {
    if (props.mode === 'normalized') {
        // Mode normalized
        let xMin = props.xMin;
        let xMax = props.xMax;
        if (xMin === null || xMax === null) {
            const values = annotation.getDistinctValuesInField(fieldName, false).map(parseFloat).filter(x => !isNaN(x));
            if (values.length > 0) {
                xMin !== null && xMin !== void 0 ? xMin : (xMin = values.reduce((a, b) => a < b ? a : b)); // xMin ??= min(values)
                xMax !== null && xMax !== void 0 ? xMax : (xMax = values.reduce((a, b) => a > b ? a : b)); // xMax ??= max(values)
            }
            else {
                xMin !== null && xMin !== void 0 ? xMin : (xMin = 0);
                xMax !== null && xMax !== void 0 ? xMax : (xMax = 1);
            }
        }
        if (xMin === xMax) {
            return x => (x < xMin ? -0.5 : x === xMin ? 0.5 : 1.5);
        }
        else {
            return x => (x - xMin) / (xMax - xMin);
        }
    }
    else {
        // Mode absolute
        return x => x;
    }
}
function makeContinuousPaletteCheckpoints(props) {
    if (props.colors.colors.every(x => Array.isArray(x))) {
        // Explicit checkpoints
        const sorted = props.colors.colors.sort((a, b) => a[1] - b[1]);
        const colors = sorted.map(color_1.Color.fromColorListEntry);
        const checkpoints = int_1.SortedArray.ofSortedArray(sorted.map(t => t[1]));
        return { colors, checkpoints };
    }
    else {
        // Auto checkpoints (linspace 0 to 1)
        const colors = props.colors.colors.map(color_1.Color.fromColorListEntry);
        const n = colors.length - 1;
        const checkpoints = int_1.SortedArray.ofSortedArray(colors.map((_, i) => i / n));
        return { colors, checkpoints };
    }
}
