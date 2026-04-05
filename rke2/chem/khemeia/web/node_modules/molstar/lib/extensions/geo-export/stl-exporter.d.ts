/**
 * Copyright (c) 2021-2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sukolsak Sakshuwong <sukolsak@stanford.edu>
 */
import { Box3D } from '../../mol-math/geometry.js';
import { RuntimeContext } from '../../mol-task/index.js';
import { MeshExporter, AddMeshInput } from './mesh-exporter.js';
export type StlData = {
    stl: Uint8Array;
};
export declare class StlExporter extends MeshExporter<StlData> {
    readonly fileExtension = "stl";
    private triangleBuffers;
    private triangleCount;
    private centerTransform;
    protected addMeshWithColors(input: AddMeshInput): Promise<void>;
    getData(): Promise<{
        stl: Uint8Array<ArrayBuffer>;
    }>;
    getBlob(ctx: RuntimeContext): Promise<Blob>;
    constructor(boundingBox: Box3D);
}
