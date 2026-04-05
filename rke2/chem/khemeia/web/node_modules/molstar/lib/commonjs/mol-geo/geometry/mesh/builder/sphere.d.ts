/**
 * Copyright (c) 2018 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Vec3 } from '../../../../mol-math/linear-algebra.js';
import { MeshBuilder } from '../mesh-builder.js';
import { Primitive } from '../../../primitive/primitive.js';
export declare function getSphere(detail: number): Primitive;
export declare function addSphere(state: MeshBuilder.State, center: Vec3, radius: number, detail: number): void;
