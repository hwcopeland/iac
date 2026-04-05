/**
 * Copyright (c) 2018-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { ComplexVisual, StructureRepresentation } from './representation.js';
import { RepresentationContext, RepresentationParamsGetter } from '../representation.js';
import { Structure } from '../../mol-model/structure.js';
import { StructureParams } from './params.js';
import { WebGLContext } from '../../mol-gl/webgl/context.js';
export declare function ComplexRepresentation<P extends StructureParams>(label: string, ctx: RepresentationContext, getParams: RepresentationParamsGetter<Structure, P>, visualCtor: (materialId: number, structure: Structure, props: PD.Values<P>, webgl?: WebGLContext) => ComplexVisual<P>): StructureRepresentation<P>;
