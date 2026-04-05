"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.getNextMaterialId = void 0;
exports.createRenderObject = createRenderObject;
exports.createRenderable = createRenderable;
const id_factory_1 = require("../mol-util/id-factory.js");
const direct_volume_1 = require("./renderable/direct-volume.js");
const mesh_1 = require("./renderable/mesh.js");
const points_1 = require("./renderable/points.js");
const lines_1 = require("./renderable/lines.js");
const spheres_1 = require("./renderable/spheres.js");
const text_1 = require("./renderable/text.js");
const texture_mesh_1 = require("./renderable/texture-mesh.js");
const image_1 = require("./renderable/image.js");
const cylinders_1 = require("./renderable/cylinders.js");
const type_helpers_1 = require("../mol-util/type-helpers.js");
const getNextId = (0, id_factory_1.idFactory)(0, 0x7FFFFFFF);
exports.getNextMaterialId = (0, id_factory_1.idFactory)(0, 0x7FFFFFFF);
//
function createRenderObject(type, values, state, materialId) {
    return { id: getNextId(), type, values, state, materialId };
}
function createRenderable(ctx, o, transparency, globals) {
    switch (o.type) {
        case 'mesh': return (0, mesh_1.MeshRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'points': return (0, points_1.PointsRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'spheres': return (0, spheres_1.SpheresRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'cylinders': return (0, cylinders_1.CylindersRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'text': return (0, text_1.TextRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'lines': return (0, lines_1.LinesRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'direct-volume': return (0, direct_volume_1.DirectVolumeRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'image': return (0, image_1.ImageRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
        case 'texture-mesh': return (0, texture_mesh_1.TextureMeshRenderable)(ctx, o.id, o.values, o.state, o.materialId, transparency, globals);
    }
    (0, type_helpers_1.assertUnreachable)(o.type);
}
