/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { ElementIndex, Model } from '../../../mol-model/structure.js';
import { CoarseElements } from '../../../mol-model/structure/model/properties/coarse.js';
import { Expression } from '../../../mol-script/language/expression.js';
import { ElementRanges } from './element-ranges.js';
import { CoarseIndicesAndSortings, IndicesAndSortings } from './indexing.js';
import { MVSAnnotationRow } from './schemas.js';
/** Return atom ranges in `model` which satisfy criteria given by `row` */
export declare function getAtomRangesForRow(row: MVSAnnotationRow, model: Model, instanceId: string, indices: IndicesAndSortings): ElementRanges | undefined;
/** Return atom ranges in `model` which satisfy criteria given by any of `rows` (atoms that satisfy more rows are still included only once) */
export declare function getAtomRangesForRows(rows: MVSAnnotationRow[], model: Model, instanceId: string, indices: IndicesAndSortings): ElementRanges;
/** Return true if `iAtom`-th atom in `model` satisfies all selection criteria given by `row`. */
export declare function atomQualifies(model: Model, iAtom: ElementIndex, row: MVSAnnotationRow): boolean;
/** Return sphere ranges in `model` which satisfy criteria given by `row` */
export declare function getSphereRangesForRow(row: MVSAnnotationRow, model: Model, instanceId: string, indices: IndicesAndSortings): ElementRanges | undefined;
/** Return sphere ranges in `model` which satisfy criteria given by any of `rows` (spheres that satisfy more rows are still included only once) */
export declare function getSphereRangesForRows(rows: MVSAnnotationRow[], model: Model, instanceId: string, indices: IndicesAndSortings): ElementRanges;
/** Return gaussian ranges in `model` which satisfy criteria given by `row` */
export declare function getGaussianRangesForRow(row: MVSAnnotationRow, model: Model, instanceId: string, indices: IndicesAndSortings): ElementRanges | undefined;
/** Return gaussian ranges in `model` which satisfy criteria given by any of `rows` (gaussians that satisfy more rows are still included only once) */
export declare function getGaussianRangesForRows(rows: MVSAnnotationRow[], model: Model, instanceId: string, indices: IndicesAndSortings): ElementRanges;
/** Return ranges of coarse elements (spheres or gaussians) which satisfy criteria given by `row` */
export declare function getCoarseElementRangesForRow(row: MVSAnnotationRow, coarseElements: CoarseElements, indices: CoarseIndicesAndSortings): ElementRanges | undefined;
/** Convert an annotation row into a MolScript expression */
export declare function rowToExpression(row: MVSAnnotationRow): Expression;
/** Convert multiple annotation rows into a MolScript expression.
 * (with union semantics, i.e. an atom qualifies if it qualifies for at least one of the rows) */
export declare function rowsToExpression(rows: readonly MVSAnnotationRow[]): Expression;
