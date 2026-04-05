/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { createRenderable } from '../renderable.js';
import { createGraphicsRenderItem } from '../webgl/render-item.js';
import { AttributeSpec, GlobalUniformSchema, InternalSchema, TextureSpec, ElementsSpec, DefineSpec, BaseSchema, UniformSpec, GlobalTextureSchema, GlobalDefineSchema } from './schema.js';
import { ImageShaderCode } from '../shader-code.js';
import { ValueCell } from '../../mol-util/value-cell.js';
export const ImageSchema = {
    ...BaseSchema,
    aGroup: AttributeSpec('float32', 1, 0),
    aPosition: AttributeSpec('float32', 3, 0),
    aUv: AttributeSpec('float32', 2, 0),
    elements: ElementsSpec('uint32'),
    uImageTexDim: UniformSpec('v2'),
    tImageTex: TextureSpec('image-uint8', 'rgba', 'ubyte', 'nearest'),
    tGroupTex: TextureSpec('image-uint8', 'rgba', 'ubyte', 'nearest'),
    tValueTex: TextureSpec('image-float32', 'alpha', 'float', 'linear'),
    uTrimType: UniformSpec('i'),
    uTrimCenter: UniformSpec('v3'),
    uTrimRotation: UniformSpec('q'),
    uTrimScale: UniformSpec('v3'),
    uTrimTransform: UniformSpec('m4'),
    uIsoLevel: UniformSpec('f'),
    /** Same as `InterpolationTypeNames` in '../../mol-geo/geometry/image/image' */
    dInterpolation: DefineSpec('string', ['nearest', 'catmulrom', 'mitchell', 'bspline']),
};
export function ImageRenderable(ctx, id, values, state, materialId, transparency, globals) {
    const schema = { ...GlobalUniformSchema, ...GlobalTextureSchema, ...GlobalDefineSchema, ...InternalSchema, ...ImageSchema };
    const renderValues = {
        ...values,
        uObjectId: ValueCell.create(id),
        dLightCount: ValueCell.create(globals.dLightCount),
        dColorMarker: ValueCell.create(globals.dColorMarker),
    };
    const shaderCode = ImageShaderCode;
    const renderItem = createGraphicsRenderItem(ctx, 'triangles', shaderCode, schema, renderValues, materialId, transparency);
    return createRenderable(renderItem, renderValues, state);
}
