/**
 * Copyright (c) 2018-2023 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Model, ResidueIndex, ElementIndex } from './model.js';
import { MoleculeType, AtomRole, PolymerType } from './model/types.js';
import { Unit } from './structure.js';
import { NumberArray } from '../../mol-util/type-helpers.js';
export declare function getCoarseBegCompId(unit: Unit.Spheres | Unit.Gaussians, element: ElementIndex): string;
export declare function getElementMoleculeType(unit: Unit, element: ElementIndex): MoleculeType;
export declare function getAtomicMoleculeType(model: Model, rI: ResidueIndex): MoleculeType;
export declare function getAtomIdForAtomRole(polymerType: PolymerType, atomRole: AtomRole): Set<string>;
export declare function getPositions(unit: Unit, indices: ArrayLike<number>): NumberArray;
