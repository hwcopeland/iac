"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.PointsSchema = void 0;
exports.PointsRenderable = PointsRenderable;
const renderable_1 = require("../renderable.js");
const render_item_1 = require("../webgl/render-item.js");
const schema_1 = require("./schema.js");
const shader_code_1 = require("../shader-code.js");
const mol_util_1 = require("../../mol-util/index.js");
exports.PointsSchema = {
    ...schema_1.BaseSchema,
    ...schema_1.SizeSchema,
    aGroup: (0, schema_1.AttributeSpec)('float32', 1, 0),
    aPosition: (0, schema_1.AttributeSpec)('float32', 3, 0),
    dPointSizeAttenuation: (0, schema_1.DefineSpec)('boolean'),
    dPointStyle: (0, schema_1.DefineSpec)('string', ['square', 'circle', 'fuzzy']),
};
function PointsRenderable(ctx, id, values, state, materialId, transparency, globals) {
    const schema = { ...schema_1.GlobalUniformSchema, ...schema_1.GlobalTextureSchema, ...schema_1.GlobalDefineSchema, ...schema_1.InternalSchema, ...exports.PointsSchema };
    const renderValues = {
        ...values,
        uObjectId: mol_util_1.ValueCell.create(id),
        dLightCount: mol_util_1.ValueCell.create(globals.dLightCount),
        dColorMarker: mol_util_1.ValueCell.create(globals.dColorMarker),
    };
    const shaderCode = shader_code_1.PointsShaderCode;
    const renderItem = (0, render_item_1.createGraphicsRenderItem)(ctx, 'points', shaderCode, schema, renderValues, materialId, transparency);
    return (0, renderable_1.createRenderable)(renderItem, renderValues, state);
}
