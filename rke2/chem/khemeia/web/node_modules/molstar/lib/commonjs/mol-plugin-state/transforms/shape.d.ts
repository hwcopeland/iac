/**
 * Copyright (c) 2021 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Mesh } from '../../mol-geo/geometry/mesh/mesh.js';
import { Box3D } from '../../mol-math/geometry.js';
import { Vec3 } from '../../mol-math/linear-algebra.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { PluginStateObject as SO } from '../objects.js';
export { BoxShape3D };
type BoxShape3D = typeof BoxShape3D;
declare const BoxShape3D: import("../../mol-state/index.js").StateTransformer<SO.Root, SO.Shape.Provider, PD.Normalize<{
    bottomLeft: Vec3;
    topRight: Vec3;
    radius: number;
    color: import("../../mol-util/color/index.js").Color;
}>>;
export declare function getBoxMesh(box: Box3D, radius: number, oldMesh?: Mesh): Mesh;
