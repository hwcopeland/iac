/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author Gianluca Tomasello <giagitom@gmail.com>
 */
import { Vec3 } from '../../../../mol-math/linear-algebra.js';
import { MeshBuilder } from '../mesh-builder.js';
import { CylinderProps } from '../../../primitive/cylinder.js';
import { Ray3D } from '../../../../mol-math/geometry/primitives/ray3d.js';
export type BasicCylinderProps = Omit<CylinderProps, 'height'>;
export declare function addSimpleCylinder(state: MeshBuilder.State, start: Vec3, end: Vec3, props: BasicCylinderProps): void;
export declare function addCylinderFromRay3D(state: MeshBuilder.State, ray: Ray3D, length: number, props: BasicCylinderProps): void;
export declare function addCylinder(state: MeshBuilder.State, start: Vec3, end: Vec3, lengthScale: number, props: BasicCylinderProps): void;
export declare function addDoubleCylinder(state: MeshBuilder.State, start: Vec3, end: Vec3, lengthScale: number, shift: Vec3, props: BasicCylinderProps): void;
export declare function addFixedCountDashedCylinder(state: MeshBuilder.State, start: Vec3, end: Vec3, lengthScale: number, segmentCount: number, stubCap: boolean, props: BasicCylinderProps): void;
