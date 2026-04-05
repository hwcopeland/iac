/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { idFactory } from '../mol-util/id-factory.js';
import { DirectVolumeRenderable } from './renderable/direct-volume.js';
import { MeshRenderable } from './renderable/mesh.js';
import { PointsRenderable } from './renderable/points.js';
import { LinesRenderable } from './renderable/lines.js';
import { SpheresRenderable } from './renderable/spheres.js';
import { TextRenderable } from './renderable/text.js';
import { TextureMeshRenderable } from './renderable/texture-mesh.js';
import { ImageRenderable } from './renderable/image.js';
import { CylindersRenderable } from './renderable/cylinders.js';
import { assertUnreachable } from '../mol-util/type-helpers.js';
const getNextId = idFactory(0, 0x7FFFFFFF);
export const getNextMaterialId = idFactory(0, 0x7FFFFFFF);
//
export function createRenderObject(type, values, state, materialId) {
    return { id: getNextId(), type, values, state, materialId };
}
export function createRenderable(ctx, o, transparency, globals) {
    switch (o.type) {
        case 'mesh': return MeshRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'points': return PointsRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'spheres': return SpheresRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'cylinders': return CylindersRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'text': return TextRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'lines': return LinesRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'direct-volume': return DirectVolumeRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'image': return ImageRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'texture-mesh': return TextureMeshRenderable(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
    }
    assertUnreachable(o.type);
}
