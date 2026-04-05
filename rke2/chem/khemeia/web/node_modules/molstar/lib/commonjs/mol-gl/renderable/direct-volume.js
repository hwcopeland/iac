"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.DirectVolumeSchema = void 0;
exports.DirectVolumeRenderable = DirectVolumeRenderable;
const renderable_1 = require("../renderable.js");
const render_item_1 = require("../webgl/render-item.js");
const schema_1 = require("./schema.js");
const shader_code_1 = require("../shader-code.js");
const mol_util_1 = require("../../mol-util/index.js");
exports.DirectVolumeSchema = {
    ...schema_1.BaseSchema,
    aPosition: (0, schema_1.AttributeSpec)('float32', 3, 0),
    elements: (0, schema_1.ElementsSpec)('uint32'),
    uBboxMin: (0, schema_1.UniformSpec)('v3'),
    uBboxMax: (0, schema_1.UniformSpec)('v3'),
    uBboxSize: (0, schema_1.UniformSpec)('v3'),
    uMaxSteps: (0, schema_1.UniformSpec)('i'),
    uStepScale: (0, schema_1.UniformSpec)('f'),
    uJumpLength: (0, schema_1.UniformSpec)('f'),
    uTransform: (0, schema_1.UniformSpec)('m4'),
    uGridDim: (0, schema_1.UniformSpec)('v3'),
    tTransferTex: (0, schema_1.TextureSpec)('image-uint8', 'alpha', 'ubyte', 'linear'),
    uTransferScale: (0, schema_1.UniformSpec)('f', 'material'),
    dGridTexType: (0, schema_1.DefineSpec)('string', ['2d', '3d']),
    uGridTexDim: (0, schema_1.UniformSpec)('v3'),
    tGridTex: (0, schema_1.TextureSpec)('texture', 'rgba', 'ubyte', 'linear'),
    uGridStats: (0, schema_1.UniformSpec)('v4'), // [min, max, mean, sigma]
    uCellDim: (0, schema_1.UniformSpec)('v3'),
    uCartnToUnit: (0, schema_1.UniformSpec)('m4'),
    uUnitToCartn: (0, schema_1.UniformSpec)('m4'),
    dPackedGroup: (0, schema_1.DefineSpec)('boolean'),
    dAxisOrder: (0, schema_1.DefineSpec)('string', ['012', '021', '102', '120', '201', '210']),
    dIgnoreLight: (0, schema_1.DefineSpec)('boolean'),
    dCelShaded: (0, schema_1.DefineSpec)('boolean'),
    dXrayShaded: (0, schema_1.DefineSpec)('string', ['off', 'on', 'inverted']),
    meta: (0, schema_1.ValueSpec)('unknown')
};
function DirectVolumeRenderable(ctx, id, values, state, materialId, transparency, globals) {
    const schema = { ...schema_1.GlobalUniformSchema, ...schema_1.GlobalTextureSchema, ...schema_1.GlobalDefineSchema, ...schema_1.InternalSchema, ...exports.DirectVolumeSchema };
    if (!ctx.isWebGL2) {
        // workaround for webgl1 limitation that loop counters need to be `const`
        schema.uMaxSteps = (0, schema_1.DefineSpec)('number');
    }
    const renderValues = {
        ...values,
        uObjectId: mol_util_1.ValueCell.create(id),
        dLightCount: mol_util_1.ValueCell.create(globals.dLightCount),
        dColorMarker: mol_util_1.ValueCell.create(globals.dColorMarker),
    };
    const shaderCode = shader_code_1.DirectVolumeShaderCode;
    const renderItem = (0, render_item_1.createGraphicsRenderItem)(ctx, 'triangles', shaderCode, schema, renderValues, materialId, transparency);
    return (0, renderable_1.createRenderable)(renderItem, renderValues, state);
}
