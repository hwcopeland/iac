"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MultilayerColorThemeName = exports.DefaultMultilayerColorThemeProps = exports.NoColor = void 0;
exports.makeMultilayerColorThemeParams = makeMultilayerColorThemeParams;
exports.makeMultilayerColorThemeProvider = makeMultilayerColorThemeProvider;
const structure_1 = require("../../../mol-model/structure.js");
const categories_1 = require("../../../mol-theme/color/categories.js");
const mol_util_1 = require("../../../mol-util/index.js");
const color_1 = require("../../../mol-util/color/index.js");
const names_1 = require("../../../mol-util/color/names.js");
const param_definition_1 = require("../../../mol-util/param-definition.js");
const string_1 = require("../../../mol-util/string.js");
const is_mvs_model_prop_1 = require("./is-mvs-model-prop.js");
const selector_1 = require("./selector.js");
/** Special value that can be used as color with null-like semantic (i.e. "no color provided").
 * By some lucky coincidence, Mol* treats -1 as white. */
exports.NoColor = (0, color_1.Color)(-1);
/** Return true if `color` is a real color, false if it is `NoColor`. */
function isValidColor(color) {
    return color >= 0;
}
const DefaultBackgroundColor = names_1.ColorNames.white;
/** Parameter definition for color theme "Multilayer" */
function makeMultilayerColorThemeParams(colorThemeRegistry, ctx) {
    const colorThemeInfo = {
        help: (value) => {
            const { name, params } = value;
            const p = colorThemeRegistry.get(name);
            const ct = p.factory({}, params);
            return { description: ct.description, legend: ct.legend };
        }
    };
    const nestedThemeTypes = colorThemeRegistry.types.filter(([name, label, category]) => name !== exports.MultilayerColorThemeName && colorThemeRegistry.get(name).isApplicable(ctx)); // Adding 'multilayer' theme itself would cause infinite recursion
    return {
        layers: param_definition_1.ParamDefinition.ObjectList({
            theme: param_definition_1.ParamDefinition.Mapped('uniform', nestedThemeTypes, name => param_definition_1.ParamDefinition.Group(colorThemeRegistry.get(name).getParams({ structure: structure_1.Structure.Empty })), colorThemeInfo),
            selection: selector_1.SelectorParams,
        }, obj => (0, string_1.stringToWords)(obj.theme.name), { description: 'A list of layers, each defining a color theme. The last listed layer is the top layer (applies first). If the top layer does not provide color for a location or its selection does not cover the location, the underneath layers will apply.' }),
        background: param_definition_1.ParamDefinition.Color(DefaultBackgroundColor, { description: 'Color for elements where no layer applies' }),
    };
}
/** Default values for `MultilayerColorThemeProps` */
exports.DefaultMultilayerColorThemeProps = { layers: [], background: DefaultBackgroundColor };
/** Return color theme that assigns colors based on a list of nested color themes (layers).
 * The last layer in the list whose selection covers the given location
 * and which provides a valid (non-negative) color value will be used.
 * If a nested theme provider has `ensureCustomProperties` methods, these will not be called automatically
 * (the caller must ensure that any required custom properties be attached). */
function makeMultilayerColorTheme(ctx, props, colorThemeRegistry) {
    const { colorLayers, granularity, preferSmoothing } = makeLayers(ctx, props, colorThemeRegistry);
    function structureElementColor(loc, isSecondary) {
        for (const layer of colorLayers) {
            const matches = !layer.elementSet || selector_1.ElementSet.has(layer.elementSet, loc);
            if (!matches)
                continue;
            const color = layer.color(loc, isSecondary);
            if (!isValidColor(color))
                continue;
            return color;
        }
        return props.background;
    }
    const auxLocation = structure_1.StructureElement.Location.create(ctx.structure);
    const color = (location, isSecondary) => {
        if (structure_1.StructureElement.Location.is(location)) {
            return structureElementColor(location, isSecondary);
        }
        else if (structure_1.Bond.isLocation(location)) {
            // this will be applied for each bond twice, to get color of each half (a* refers to the adjacent atom, b* to the opposite atom)
            auxLocation.unit = location.aUnit;
            auxLocation.element = location.aUnit.elements[location.aIndex];
            return structureElementColor(auxLocation, isSecondary);
        }
        return props.background;
    };
    return {
        factory: (ctx_, props_) => makeMultilayerColorTheme(ctx_, props_, colorThemeRegistry),
        granularity,
        preferSmoothing,
        color: color,
        props: props,
        description: 'Combines colors from multiple color themes.',
    };
}
const GRAN_INSTANCE = 1, GRAN_GROUP = 2, GRAN_VERTEX = 4;
const granularityFlagsFromName = {
    'uniform': 0,
    'instance': GRAN_INSTANCE,
    'group': GRAN_GROUP,
    'groupInstance': GRAN_GROUP | GRAN_INSTANCE,
    'vertex': GRAN_VERTEX,
    'vertexInstance': GRAN_VERTEX | GRAN_INSTANCE,
};
function granularityNameFromFlags(flags) {
    if (flags & GRAN_VERTEX)
        return flags & GRAN_INSTANCE ? 'vertexInstance' : 'vertex';
    if (flags & GRAN_GROUP)
        return flags & GRAN_INSTANCE ? 'groupInstance' : 'group';
    return flags & GRAN_INSTANCE ? 'instance' : 'uniform';
}
function makeLayers(ctx, props, colorThemeRegistry) {
    var _a;
    const colorLayers = [];
    let granularityFlags = 0;
    let preferSmoothing = false;
    for (let i = props.layers.length - 1; i >= 0; i--) { // iterate from the end to get top layer first, bottom layer last
        const layer = props.layers[i];
        const themeProvider = colorThemeRegistry.get(layer.theme.name);
        if (!themeProvider) {
            console.warn(`Skipping color theme '${layer.theme.name}', cannot find it in registry.`);
            continue;
        }
        if ((_a = themeProvider.ensureCustomProperties) === null || _a === void 0 ? void 0 : _a.attach) {
            console.warn(`Multilayer color theme: layer "${themeProvider.name}" has ensureCustomProperties.attach method, but Multilayer color theme does not call it. If the layer does not work, make sure you call ensureCustomProperties.attach somewhere.`);
        }
        const theme = themeProvider.factory(ctx, layer.theme.params);
        switch (theme.granularity) {
            case 'uniform':
            case 'instance':
            case 'group':
            case 'groupInstance':
            case 'vertex':
            case 'vertexInstance':
                let elementSet;
                let selectionGranularity;
                if (!ctx.structure) {
                    elementSet = {};
                    selectionGranularity = 'uniform';
                }
                else if ((0, selector_1.isSelectorAll)(layer.selection)) {
                    // Treating 'all' specially for performance reasons (it's expected to be used most often)
                    elementSet = undefined;
                    selectionGranularity = 'uniform';
                }
                else {
                    const substructure = (0, selector_1.substructureFromSelector)(ctx.structure, layer.selection);
                    elementSet = selector_1.ElementSet.fromStructure(substructure);
                    selectionGranularity = getSubstructureGranularity(ctx.structure, substructure);
                }
                colorLayers.push({ elementSet, color: theme.color });
                granularityFlags |= granularityFlagsFromName[selectionGranularity];
                granularityFlags |= granularityFlagsFromName[theme.granularity];
                break;
            default:
                console.warn(`Skipping color theme '${layer.theme.name}', cannot process granularity '${theme.granularity}'`);
        }
        if (theme.preferSmoothing)
            preferSmoothing = true;
    }
    return { colorLayers, granularity: granularityNameFromFlags(granularityFlags), preferSmoothing };
}
function getSubstructureGranularity(parent, substructure) {
    var _a, _b, _c;
    const parentCounts = {};
    for (const unit of parent.units) {
        const instance = unit.conformation.operator.instanceId;
        (_a = parentCounts[instance]) !== null && _a !== void 0 ? _a : (parentCounts[instance] = 0);
        parentCounts[instance] += unit.elements.length;
    }
    const childCounts = {};
    const elementsPerInstance = {};
    for (const unit of substructure.units) {
        const instance = unit.conformation.operator.instanceId;
        (_b = childCounts[instance]) !== null && _b !== void 0 ? _b : (childCounts[instance] = 0);
        childCounts[instance] += unit.elements.length;
        ((_c = elementsPerInstance[instance]) !== null && _c !== void 0 ? _c : (elementsPerInstance[instance] = {}))[unit.invariantId] = unit.elements;
    }
    const parentInstances = Object.keys(parentCounts);
    const childInstances = Object.keys(childCounts);
    const groupGranularity = !childInstances.every(inst => childCounts[inst] === parentCounts[inst]);
    let instanceGranularity;
    if (childInstances.length === 0) {
        instanceGranularity = false;
    }
    else if (childInstances.length < parentInstances.length) {
        instanceGranularity = true;
    }
    else {
        instanceGranularity = false;
        for (let i = 1; i < childInstances.length; i++) {
            if (!(0, mol_util_1.deepEqual)(elementsPerInstance[childInstances[0]], elementsPerInstance[childInstances[i]])) {
                instanceGranularity = true;
                break;
            }
        }
    }
    if (groupGranularity)
        return instanceGranularity ? 'groupInstance' : 'group';
    else
        return instanceGranularity ? 'instance' : 'uniform';
}
/** Unique name for "Multilayer" color theme */
exports.MultilayerColorThemeName = 'mvs-multilayer';
/** A thingy that is needed to register color theme "Multilayer" */
function makeMultilayerColorThemeProvider(colorThemeRegistry) {
    return {
        name: exports.MultilayerColorThemeName,
        label: 'MVS Multi-layer',
        category: categories_1.ColorThemeCategory.Misc,
        factory: (ctx, props) => makeMultilayerColorTheme(ctx, props, colorThemeRegistry),
        getParams: (ctx) => makeMultilayerColorThemeParams(colorThemeRegistry, ctx),
        defaultValues: exports.DefaultMultilayerColorThemeProps,
        isApplicable: (ctx) => !!ctx.structure && (0, is_mvs_model_prop_1.isMVSStructure)(ctx.structure),
    };
}
