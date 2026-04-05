/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Color } from '../../mol-util/color/index.js';
import { ShapeGroup } from '../../mol-model/shape.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ColorThemeCategory } from './categories.js';
const DefaultColor = Color(0xCCCCCC);
const Description = 'Assigns colors as defined by the shape object.';
export const ShapeGroupColorThemeParams = {};
export function getShapeGroupColorThemeParams(ctx) {
    return ShapeGroupColorThemeParams; // TODO return copy
}
export function ShapeGroupColorTheme(ctx, props) {
    return {
        factory: ShapeGroupColorTheme,
        granularity: 'groupInstance',
        color: (location) => {
            if (ShapeGroup.isLocation(location)) {
                return location.shape.getColor(location.group, location.instance);
            }
            return DefaultColor;
        },
        props,
        description: Description
    };
}
export const ShapeGroupColorThemeProvider = {
    name: 'shape-group',
    label: 'Shape Group',
    category: ColorThemeCategory.Misc,
    factory: ShapeGroupColorTheme,
    getParams: getShapeGroupColorThemeParams,
    defaultValues: PD.getDefaultValues(ShapeGroupColorThemeParams),
    isApplicable: (ctx) => !!ctx.shape
};
