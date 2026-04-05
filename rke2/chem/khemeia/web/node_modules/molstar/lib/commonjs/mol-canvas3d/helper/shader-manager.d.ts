/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Scene } from '../../mol-gl/scene.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { GraphicsRenderVariant } from '../../mol-gl/webgl/render-item.js';
import { IlluminationProps } from '../passes/illumination.js';
import { MarkingProps } from '../passes/marking.js';
import { PostprocessingProps } from '../passes/postprocessing.js';
export type ShaderManagerProps = {
    marking: MarkingProps;
    postprocessing: PostprocessingProps;
    illumination: IlluminationProps;
};
export declare class ShaderManager {
    private readonly webgl;
    private readonly scene;
    static ensureRequired(webgl: WebGLContext, scene: Scene, p: ShaderManagerProps): void;
    private readonly required;
    constructor(webgl: WebGLContext, scene: Scene);
    updateRequired(p: ShaderManagerProps): void;
    finalizeRequired(isSynchronous?: boolean): boolean;
    finalize(variants?: GraphicsRenderVariant[], isSynchronous?: boolean): boolean;
}
