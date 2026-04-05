"use strict";
/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.addAxes = addAxes;
const linear_algebra_1 = require("../../../../mol-math/linear-algebra.js");
const mesh_builder_1 = require("../mesh-builder.js");
const geometry_1 = require("../../../../mol-math/geometry.js");
const cage_1 = require("../../../primitive/cage.js");
const tmpVec = (0, linear_algebra_1.Vec3)();
const tmpMatrix = linear_algebra_1.Mat4.identity();
const tmpVertices = new Float32Array(6 * 3);
const tmpEdges = new Uint8Array([0, 1, 2, 3, 4, 5]);
function addAxes(state, axes, radiusScale, detail, radialSegments) {
    const { origin, dirA, dirB, dirC } = axes;
    linear_algebra_1.Vec3.add(tmpVec, origin, dirA);
    linear_algebra_1.Vec3.toArray(linear_algebra_1.Vec3.add(tmpVec, origin, dirA), tmpVertices, 0);
    linear_algebra_1.Vec3.toArray(linear_algebra_1.Vec3.sub(tmpVec, origin, dirA), tmpVertices, 3);
    linear_algebra_1.Vec3.toArray(linear_algebra_1.Vec3.add(tmpVec, origin, dirB), tmpVertices, 6);
    linear_algebra_1.Vec3.toArray(linear_algebra_1.Vec3.sub(tmpVec, origin, dirB), tmpVertices, 9);
    linear_algebra_1.Vec3.toArray(linear_algebra_1.Vec3.add(tmpVec, origin, dirC), tmpVertices, 12);
    linear_algebra_1.Vec3.toArray(linear_algebra_1.Vec3.sub(tmpVec, origin, dirC), tmpVertices, 15);
    const cage = (0, cage_1.createCage)(tmpVertices, tmpEdges);
    const volume = geometry_1.Axes3D.volume(axes);
    const radius = (Math.cbrt(volume) / 300) * radiusScale;
    mesh_builder_1.MeshBuilder.addCage(state, tmpMatrix, cage, radius, detail, radialSegments);
}
