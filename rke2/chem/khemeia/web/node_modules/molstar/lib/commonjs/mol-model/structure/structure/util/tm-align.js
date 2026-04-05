"use strict";
/**
 * Copyright (c) 2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Diego del Alamo <diego.delalamo@gmail.com>
 *
 * Structure-level TM-align wrapper
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.tmAlign = tmAlign;
exports.tmAlignMultiple = tmAlignMultiple;
const mat4_1 = require("../../../../mol-math/linear-algebra/3d/mat4.js");
const tm_align_1 = require("../../../../mol-math/linear-algebra/3d/tm-align.js");
const element_1 = require("../element.js");
const superposition_1 = require("./superposition.js");
/**
 * Perform TM-align on two structure element loci.
 * Aligns structure B onto structure A (A is the reference).
 *
 * @param a Reference structure loci (will not be transformed)
 * @param b Mobile structure loci (transformation returned)
 * @returns TM-align result with transformation, scores, and alignment
 */
function tmAlign(a, b) {
    const lenA = element_1.StructureElement.Loci.size(a);
    const lenB = element_1.StructureElement.Loci.size(b);
    if (lenA === 0 || lenB === 0) {
        return {
            bTransform: mat4_1.Mat4.identity(),
            tmScoreA: 0,
            tmScoreB: 0,
            rmsd: 0,
            alignedLength: 0,
            sequenceIdentity: 0,
            alignmentA: [],
            alignmentB: []
        };
    }
    const posA = (0, superposition_1.getPositionTable)(a, lenA);
    const posB = (0, superposition_1.getPositionTable)(b, lenB);
    return tm_align_1.TMAlign.compute({ a: posA, b: posB });
}
/**
 * Perform TM-align on multiple structure element loci.
 * The first structure is used as the reference; all others are aligned to it.
 *
 * @param xs Array of structure element loci (first is reference)
 * @returns Array of TM-align results (length = xs.length - 1)
 */
function tmAlignMultiple(xs) {
    const results = [];
    if (xs.length < 2)
        return results;
    const refLoci = xs[0];
    const lenRef = element_1.StructureElement.Loci.size(refLoci);
    const posRef = (0, superposition_1.getPositionTable)(refLoci, lenRef);
    for (let i = 1; i < xs.length; i++) {
        const mobileLoci = xs[i];
        const lenMobile = element_1.StructureElement.Loci.size(mobileLoci);
        const posMobile = (0, superposition_1.getPositionTable)(mobileLoci, lenMobile);
        results.push(tm_align_1.TMAlign.compute({ a: posRef, b: posMobile }));
    }
    return results;
}
