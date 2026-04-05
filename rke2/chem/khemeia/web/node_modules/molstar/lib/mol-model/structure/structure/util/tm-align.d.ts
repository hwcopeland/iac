/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Diego del Alamo <diego.delalamo@gmail.com>
 *
 * Structure-level TM-align wrapper
 */
import { TMAlign } from '../../../../mol-math/linear-algebra/3d/tm-align.js';
import { StructureElement } from '../element.js';
export { tmAlign, tmAlignMultiple };
export type TMAlignResult = TMAlign.Result;
/**
 * Perform TM-align on two structure element loci.
 * Aligns structure B onto structure A (A is the reference).
 *
 * @param a Reference structure loci (will not be transformed)
 * @param b Mobile structure loci (transformation returned)
 * @returns TM-align result with transformation, scores, and alignment
 */
declare function tmAlign(a: StructureElement.Loci, b: StructureElement.Loci): TMAlignResult;
/**
 * Perform TM-align on multiple structure element loci.
 * The first structure is used as the reference; all others are aligned to it.
 *
 * @param xs Array of structure element loci (first is reference)
 * @returns Array of TM-align results (length = xs.length - 1)
 */
declare function tmAlignMultiple(xs: StructureElement.Loci[]): TMAlignResult[];
