/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Fred Ludlow <fred.ludlow@gmail.com>
 */
import { Mesh } from '../../geometry/mesh/mesh.js';
import { Lines } from '../../geometry/lines/lines.js';
export interface MarchingCubesBuilder<T> {
    addVertex(x: number, y: number, z: number): number;
    addNormal(x: number, y: number, z: number): void;
    addGroup(group: number): void;
    addTriangle(vertList: number[], a: number, b: number, c: number, edgeFilter: number): void;
    get(): T;
}
export declare function MarchingCubesMeshBuilder(vertexChunkSize: number, mesh?: Mesh): MarchingCubesBuilder<Mesh>;
export declare function MarchingCubesLinesBuilder(vertexChunkSize: number, lines?: Lines): MarchingCubesBuilder<Lines>;
