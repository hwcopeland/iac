/**
 * Copyright (c) 2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Vec2 } from '../../mol-math/linear-algebra/3d/vec2.js';
import { ValueCell } from '../../mol-util/value-cell.js';
import { createComputeRenderable } from '../renderable.js';
import { TextureSpec, UniformSpec } from '../renderable/schema.js';
import { ShaderCode } from '../shader-code.js';
import { hiZ_frag } from '../shader/hi-z.frag.js';
import { quad_vert } from '../shader/quad.vert.js';
import { createComputeRenderItem } from '../webgl/render-item.js';
import { QuadSchema, QuadValues } from './util.js';
const HiZSchema = {
    ...QuadSchema,
    tPreviousLevel: TextureSpec('texture', 'alpha', 'float', 'nearest'),
    uInvSize: UniformSpec('v2'),
    uOffset: UniformSpec('v2'),
};
const HiZShaderCode = ShaderCode('hi-z', quad_vert, hiZ_frag);
export function createHiZRenderable(ctx, previousLevel) {
    const values = {
        ...QuadValues,
        tPreviousLevel: ValueCell.create(previousLevel),
        uInvSize: ValueCell.create(Vec2()),
        uOffset: ValueCell.create(Vec2()),
    };
    const schema = { ...HiZSchema };
    const renderItem = createComputeRenderItem(ctx, 'triangles', HiZShaderCode, schema, values);
    return createComputeRenderable(renderItem, values);
}
