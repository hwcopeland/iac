/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { RenderableState, Renderable } from './renderable.js';
import { WebGLContext } from './webgl/context.js';
import { DirectVolumeValues } from './renderable/direct-volume.js';
import { MeshValues } from './renderable/mesh.js';
import { PointsValues } from './renderable/points.js';
import { LinesValues } from './renderable/lines.js';
import { SpheresValues } from './renderable/spheres.js';
import { TextValues } from './renderable/text.js';
import { TextureMeshValues } from './renderable/texture-mesh.js';
import { ImageValues } from './renderable/image.js';
import { CylindersValues } from './renderable/cylinders.js';
import { Transparency } from './webgl/render-item.js';
import { GlobalDefines } from './renderable/schema.js';
export declare const getNextMaterialId: () => number;
export interface GraphicsRenderObject<T extends RenderObjectType = RenderObjectType> {
    readonly id: number;
    readonly type: T;
    readonly values: RenderObjectValues<T>;
    readonly state: RenderableState;
    readonly materialId: number;
}
export type RenderObjectType = 'mesh' | 'points' | 'spheres' | 'cylinders' | 'text' | 'lines' | 'direct-volume' | 'image' | 'texture-mesh';
export type RenderObjectValues<T extends RenderObjectType> = T extends 'mesh' ? MeshValues : T extends 'points' ? PointsValues : T extends 'spheres' ? SpheresValues : T extends 'cylinders' ? CylindersValues : T extends 'text' ? TextValues : T extends 'lines' ? LinesValues : T extends 'direct-volume' ? DirectVolumeValues : T extends 'image' ? ImageValues : T extends 'texture-mesh' ? TextureMeshValues : never;
export declare function createRenderObject<T extends RenderObjectType>(type: T, values: RenderObjectValues<T>, state: RenderableState, materialId: number): GraphicsRenderObject<T>;
export declare function createRenderable<T extends RenderObjectType>(ctx: WebGLContext, o: GraphicsRenderObject<T>, transparency: Transparency, globals: GlobalDefines): Renderable<any>;
