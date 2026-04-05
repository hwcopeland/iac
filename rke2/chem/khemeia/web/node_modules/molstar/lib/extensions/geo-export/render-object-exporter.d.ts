/**
 * Copyright (c) 2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sukolsak Sakshuwong <sukolsak@stanford.edu>
 */
import { GraphicsRenderObject } from '../../mol-gl/render-object.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
import { RuntimeContext } from '../../mol-task/index.js';
export type RenderObjectExportData = {
    [k: string]: string | Uint8Array | ArrayBuffer | undefined;
};
export interface RenderObjectExporter<D extends RenderObjectExportData> {
    readonly fileExtension: string;
    add(renderObject: GraphicsRenderObject, webgl: WebGLContext, ctx: RuntimeContext): Promise<void> | undefined;
    getData(ctx: RuntimeContext): Promise<D>;
    getBlob(ctx: RuntimeContext): Promise<Blob>;
}
