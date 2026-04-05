/**
 * Copyright (c) 2022-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Color } from '../../mol-util/color/index.js';
import { StructureElement, Bond, Structure } from '../../mol-model/structure.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { getPaletteParams, getPalette } from '../../mol-util/color/palette.js';
import { ColorThemeCategory } from './categories.js';
const DefaultColor = Color(0xCCCCCC);
const Description = 'Gives every structure a unique color based on its index.';
export const StructureIndexColorThemeParams = {
    ...getPaletteParams({ type: 'colors', colorList: 'many-distinct' }),
};
export function getStructureIndexColorThemeParams(ctx) {
    return PD.clone(StructureIndexColorThemeParams);
}
export function StructureIndexColorTheme(ctx, props) {
    var _a;
    let color;
    let legend;
    let contextHash = -1;
    if (ctx.structure) {
        const size = ((_a = Structure.MaxIndex.get(ctx.structure).value) !== null && _a !== void 0 ? _a : -1) + 1;
        contextHash = size;
        const palette = getPalette(size, props);
        legend = palette.legend;
        color = (location) => {
            if (StructureElement.Location.is(location)) {
                return palette.color(Structure.Index.get(location.structure).value || 0);
            }
            else if (Bond.isLocation(location)) {
                return palette.color(Structure.Index.get(location.aStructure).value || 0);
            }
            return DefaultColor;
        };
    }
    else {
        color = () => DefaultColor;
    }
    return {
        factory: StructureIndexColorTheme,
        granularity: 'instance',
        color,
        props,
        contextHash,
        description: Description,
        legend
    };
}
export const StructureIndexColorThemeProvider = {
    name: 'structure-index',
    label: 'Structure Index',
    category: ColorThemeCategory.Chain,
    factory: StructureIndexColorTheme,
    getParams: getStructureIndexColorThemeParams,
    defaultValues: PD.getDefaultValues(StructureIndexColorThemeParams),
    isApplicable: (ctx) => !!ctx.structure && ctx.structure.elementCount > 0
};
