"use strict";
/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.VolumeInstanceColorThemeProvider = exports.VolumeInstanceColorThemeParams = void 0;
exports.getVolumeInstanceColorThemeParams = getVolumeInstanceColorThemeParams;
exports.VolumeInstanceColorTheme = VolumeInstanceColorTheme;
const color_1 = require("../../mol-util/color/index.js");
const param_definition_1 = require("../../mol-util/param-definition.js");
const palette_1 = require("../../mol-util/color/palette.js");
const lists_1 = require("../../mol-util/color/lists.js");
const categories_1 = require("./categories.js");
const volume_1 = require("../../mol-model/volume/volume.js");
const DefaultList = 'dark-2';
const DefaultColor = (0, color_1.Color)(0xCCCCCC);
const Description = 'Gives every volume instance a unique color based on the position (index) of the instance in the list of instances of the volume.';
exports.VolumeInstanceColorThemeParams = {
    ...(0, palette_1.getPaletteParams)({ type: 'colors', colorList: DefaultList }),
};
function getVolumeInstanceColorThemeParams(ctx) {
    const params = param_definition_1.ParamDefinition.clone(exports.VolumeInstanceColorThemeParams);
    if (ctx.volume) {
        if (ctx.volume.instances.length > lists_1.ColorLists[DefaultList].list.length) {
            params.palette.defaultValue.name = 'colors';
            params.palette.defaultValue.params = {
                ...params.palette.defaultValue.params,
                list: { kind: 'interpolate', colors: (0, lists_1.getColorListFromName)(DefaultList).list }
            };
        }
    }
    return params;
}
function VolumeInstanceColorTheme(ctx, props) {
    let color;
    let legend;
    if (ctx.volume) {
        const palette = (0, palette_1.getPalette)(ctx.volume.instances.length, props);
        legend = palette.legend;
        const isLocation = volume_1.Volume.Cell.isLocation;
        color = (location) => {
            if (isLocation(location)) {
                return palette.color(location.instance);
            }
            return DefaultColor;
        };
    }
    else {
        color = () => DefaultColor;
    }
    return {
        factory: VolumeInstanceColorTheme,
        granularity: 'instance',
        color,
        props,
        description: Description,
        legend
    };
}
exports.VolumeInstanceColorThemeProvider = {
    name: 'volume-instance',
    label: 'Volume Instance',
    category: categories_1.ColorThemeCategory.Misc,
    factory: VolumeInstanceColorTheme,
    getParams: getVolumeInstanceColorThemeParams,
    defaultValues: param_definition_1.ParamDefinition.getDefaultValues(exports.VolumeInstanceColorThemeParams),
    isApplicable: (ctx) => !!ctx.volume
};
