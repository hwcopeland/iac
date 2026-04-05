/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { QuadSchema, QuadValues } from '../../mol-gl/compute/util.js';
import { createComputeRenderable } from '../../mol-gl/renderable.js';
import { DefineSpec, TextureSpec, UniformSpec } from '../../mol-gl/renderable/schema.js';
import { ShaderCode } from '../../mol-gl/shader-code.js';
import { createComputeRenderItem } from '../../mol-gl/webgl/render-item.js';
import { Vec2 } from '../../mol-math/linear-algebra.js';
import { ValueCell } from '../../mol-util/index.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { quad_vert } from '../../mol-gl/shader/quad.vert.js';
import { isTimingMode } from '../../mol-util/debug.js';
import { cas_frag } from '../../mol-gl/shader/cas.frag.js';
export const CasParams = {
    sharpness: PD.Numeric(0.5, { min: 0, max: 1, step: 0.05 }),
    denoise: PD.Boolean(true),
};
export class CasPass {
    constructor(webgl, input) {
        this.webgl = webgl;
        this.renderable = getCasRenderable(webgl, input);
    }
    updateState(viewport) {
        const { gl, state } = this.webgl;
        state.enable(gl.SCISSOR_TEST);
        state.disable(gl.BLEND);
        state.disable(gl.DEPTH_TEST);
        state.depthMask(false);
        const { x, y, width, height } = viewport;
        state.viewport(x, y, width, height);
        state.scissor(x, y, width, height);
        state.clearColor(0, 0, 0, 1);
        gl.clear(gl.COLOR_BUFFER_BIT);
    }
    setSize(width, height) {
        ValueCell.update(this.renderable.values.uTexSizeInv, Vec2.set(this.renderable.values.uTexSizeInv.ref.value, 1 / width, 1 / height));
    }
    update(input, props) {
        const { values } = this.renderable;
        const { sharpness, denoise } = props;
        let needsUpdate = false;
        if (values.tColor.ref.value !== input) {
            ValueCell.update(this.renderable.values.tColor, input);
            needsUpdate = true;
        }
        ValueCell.updateIfChanged(values.uSharpness, 2 - 2 * Math.pow(sharpness, 0.25));
        if (values.dDenoise.ref.value !== denoise)
            needsUpdate = true;
        ValueCell.updateIfChanged(values.dDenoise, denoise);
        if (needsUpdate) {
            this.renderable.update();
        }
    }
    render(viewport, target) {
        if (isTimingMode)
            this.webgl.timer.mark('CasPass.render');
        if (target) {
            target.bind();
        }
        else {
            this.webgl.bindDrawingBuffer();
        }
        this.updateState(viewport);
        this.renderable.render();
        if (isTimingMode)
            this.webgl.timer.markEnd('CasPass.render');
    }
}
//
const CasSchema = {
    ...QuadSchema,
    tColor: TextureSpec('texture', 'rgba', 'ubyte', 'linear'),
    uTexSizeInv: UniformSpec('v2'),
    uSharpness: UniformSpec('f'),
    dDenoise: DefineSpec('boolean'),
};
const CasShaderCode = ShaderCode('cas', quad_vert, cas_frag);
function getCasRenderable(ctx, colorTexture) {
    const width = colorTexture.getWidth();
    const height = colorTexture.getHeight();
    const values = {
        ...QuadValues,
        tColor: ValueCell.create(colorTexture),
        uTexSizeInv: ValueCell.create(Vec2.create(1 / width, 1 / height)),
        uSharpness: ValueCell.create(0.5),
        dDenoise: ValueCell.create(true),
    };
    const schema = { ...CasSchema };
    const renderItem = createComputeRenderItem(ctx, 'triangles', CasShaderCode, schema, values);
    return createComputeRenderable(renderItem, values);
}
