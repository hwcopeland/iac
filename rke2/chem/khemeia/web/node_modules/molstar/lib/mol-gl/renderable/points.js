/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { createRenderable } from '../renderable.js';
import { createGraphicsRenderItem } from '../webgl/render-item.js';
import { GlobalUniformSchema, BaseSchema, AttributeSpec, DefineSpec, InternalSchema, SizeSchema, GlobalTextureSchema, GlobalDefineSchema } from './schema.js';
import { PointsShaderCode } from '../shader-code.js';
import { ValueCell } from '../../mol-util/index.js';
export const PointsSchema = {
    ...BaseSchema,
    ...SizeSchema,
    aGroup: AttributeSpec('float32', 1, 0),
    aPosition: AttributeSpec('float32', 3, 0),
    dPointSizeAttenuation: DefineSpec('boolean'),
    dPointStyle: DefineSpec('string', ['square', 'circle', 'fuzzy']),
};
export function PointsRenderable(ctx, id, values, state, materialId, transparency, globals) {
    const schema = { ...GlobalUniformSchema, ...GlobalTextureSchema, ...GlobalDefineSchema, ...InternalSchema, ...PointsSchema };
    const renderValues = {
        ...values,
        uObjectId: ValueCell.create(id),
        dLightCount: ValueCell.create(globals.dLightCount),
        dColorMarker: ValueCell.create(globals.dColorMarker),
    };
    const shaderCode = PointsShaderCode;
    const renderItem = createGraphicsRenderItem(ctx, 'points', shaderCode, schema, renderValues, materialId, transparency);
    return createRenderable(renderItem, renderValues, state);
}
