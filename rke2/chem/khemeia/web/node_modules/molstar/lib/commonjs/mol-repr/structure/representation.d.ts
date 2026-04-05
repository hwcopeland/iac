/**
 * Copyright (c) 2018-2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Structure } from '../../mol-model/structure.js';
import { StructureUnitTransforms } from '../../mol-model/structure/structure/util/unit-transforms.js';
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Representation, RepresentationProps, RepresentationProvider } from '../representation.js';
export interface StructureRepresentationState extends Representation.State {
    unitTransforms: StructureUnitTransforms | null;
    unitTransformsVersion: number;
}
export declare const StructureRepresentationStateBuilder: Representation.StateBuilder<StructureRepresentationState>;
export interface StructureRepresentation<P extends RepresentationProps = {}> extends Representation<Structure, P, StructureRepresentationState> {
}
export type StructureRepresentationProvider<P extends PD.Params, Id extends string = string> = RepresentationProvider<Structure, P, StructureRepresentationState, Id>;
export declare function StructureRepresentationProvider<P extends PD.Params, Id extends string>(p: StructureRepresentationProvider<P, Id>): StructureRepresentationProvider<P, Id>;
export { ComplexRepresentation } from './complex-representation.js';
export { ComplexVisual } from './complex-visual.js';
export { UnitsRepresentation } from './units-representation.js';
export { UnitsVisual } from './units-visual.js';
