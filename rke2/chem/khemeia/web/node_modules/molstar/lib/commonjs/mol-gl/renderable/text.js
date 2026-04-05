"use strict";
/**
 * Copyright (c) 2019-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.TextSchema = void 0;
exports.TextRenderable = TextRenderable;
const renderable_1 = require("../renderable.js");
const render_item_1 = require("../webgl/render-item.js");
const schema_1 = require("./schema.js");
const shader_code_1 = require("../shader-code.js");
const mol_util_1 = require("../../mol-util/index.js");
exports.TextSchema = {
    ...schema_1.BaseSchema,
    ...schema_1.SizeSchema,
    aGroup: (0, schema_1.AttributeSpec)('float32', 1, 0),
    aPosition: (0, schema_1.AttributeSpec)('float32', 3, 0),
    aMapping: (0, schema_1.AttributeSpec)('float32', 2, 0),
    aDepth: (0, schema_1.AttributeSpec)('float32', 1, 0),
    elements: (0, schema_1.ElementsSpec)('uint32'),
    aTexCoord: (0, schema_1.AttributeSpec)('float32', 2, 0),
    tFont: (0, schema_1.TextureSpec)('image-uint8', 'alpha', 'ubyte', 'linear'),
    padding: (0, schema_1.ValueSpec)('number'),
    uBorderWidth: (0, schema_1.UniformSpec)('f', 'material'),
    uBorderColor: (0, schema_1.UniformSpec)('v3', 'material'),
    uOffsetX: (0, schema_1.UniformSpec)('f', 'material'),
    uOffsetY: (0, schema_1.UniformSpec)('f', 'material'),
    uOffsetZ: (0, schema_1.UniformSpec)('f', 'material'),
    uBackgroundColor: (0, schema_1.UniformSpec)('v3', 'material'),
    uBackgroundOpacity: (0, schema_1.UniformSpec)('f', 'material'),
};
function TextRenderable(ctx, id, values, state, materialId, transparency, globals) {
    const schema = { ...schema_1.GlobalUniformSchema, ...schema_1.GlobalTextureSchema, ...schema_1.GlobalDefineSchema, ...schema_1.InternalSchema, ...exports.TextSchema };
    const renderValues = {
        ...values,
        uObjectId: mol_util_1.ValueCell.create(id),
        dLightCount: mol_util_1.ValueCell.create(globals.dLightCount),
        dColorMarker: mol_util_1.ValueCell.create(globals.dColorMarker),
    };
    const shaderCode = shader_code_1.TextShaderCode;
    const renderItem = (0, render_item_1.createGraphicsRenderItem)(ctx, 'triangles', shaderCode, schema, renderValues, materialId, transparency);
    return (0, renderable_1.createRenderable)(renderItem, renderValues, state);
}
