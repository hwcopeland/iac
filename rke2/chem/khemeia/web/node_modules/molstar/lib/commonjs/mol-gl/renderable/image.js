"use strict";
/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ImageSchema = void 0;
exports.ImageRenderable = ImageRenderable;
const renderable_1 = require("../renderable.js");
const render_item_1 = require("../webgl/render-item.js");
const schema_1 = require("./schema.js");
const shader_code_1 = require("../shader-code.js");
const value_cell_1 = require("../../mol-util/value-cell.js");
exports.ImageSchema = {
    ...schema_1.BaseSchema,
    aGroup: (0, schema_1.AttributeSpec)('float32', 1, 0),
    aPosition: (0, schema_1.AttributeSpec)('float32', 3, 0),
    aUv: (0, schema_1.AttributeSpec)('float32', 2, 0),
    elements: (0, schema_1.ElementsSpec)('uint32'),
    uImageTexDim: (0, schema_1.UniformSpec)('v2'),
    tImageTex: (0, schema_1.TextureSpec)('image-uint8', 'rgba', 'ubyte', 'nearest'),
    tGroupTex: (0, schema_1.TextureSpec)('image-uint8', 'rgba', 'ubyte', 'nearest'),
    tValueTex: (0, schema_1.TextureSpec)('image-float32', 'alpha', 'float', 'linear'),
    uTrimType: (0, schema_1.UniformSpec)('i'),
    uTrimCenter: (0, schema_1.UniformSpec)('v3'),
    uTrimRotation: (0, schema_1.UniformSpec)('q'),
    uTrimScale: (0, schema_1.UniformSpec)('v3'),
    uTrimTransform: (0, schema_1.UniformSpec)('m4'),
    uIsoLevel: (0, schema_1.UniformSpec)('f'),
    /** Same as `InterpolationTypeNames` in '../../mol-geo/geometry/image/image' */
    dInterpolation: (0, schema_1.DefineSpec)('string', ['nearest', 'catmulrom', 'mitchell', 'bspline']),
};
function ImageRenderable(ctx, id, values, state, materialId, transparency, globals) {
    const schema = { ...schema_1.GlobalUniformSchema, ...schema_1.GlobalTextureSchema, ...schema_1.GlobalDefineSchema, ...schema_1.InternalSchema, ...exports.ImageSchema };
    const renderValues = {
        ...values,
        uObjectId: value_cell_1.ValueCell.create(id),
        dLightCount: value_cell_1.ValueCell.create(globals.dLightCount),
        dColorMarker: value_cell_1.ValueCell.create(globals.dColorMarker),
    };
    const shaderCode = shader_code_1.ImageShaderCode;
    const renderItem = (0, render_item_1.createGraphicsRenderItem)(ctx, 'triangles', shaderCode, schema, renderValues, materialId, transparency);
    return (0, renderable_1.createRenderable)(renderItem, renderValues, state);
}
