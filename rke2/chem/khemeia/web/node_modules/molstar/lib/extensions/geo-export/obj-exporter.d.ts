/**
 * Copyright (c) 2021-2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Sukolsak Sakshuwong <sukolsak@stanford.edu>
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Box3D } from '../../mol-math/geometry.js';
import { RuntimeContext } from '../../mol-task/index.js';
import { MeshExporter, AddMeshInput } from './mesh-exporter.js';
export type ObjData = {
    obj: string;
    mtl: string;
};
export declare class ObjExporter extends MeshExporter<ObjData> {
    private filename;
    readonly fileExtension = "zip";
    private obj;
    private mtl;
    private vertexOffset;
    private currentColor;
    private currentAlpha;
    private materialSet;
    private centerTransform;
    private updateMaterial;
    protected addMeshWithColors(input: AddMeshInput): Promise<void>;
    getData(): Promise<{
        obj: string;
        mtl: string;
    }>;
    getBlob(ctx: RuntimeContext): Promise<Blob>;
    constructor(filename: string, boundingBox: Box3D);
}
