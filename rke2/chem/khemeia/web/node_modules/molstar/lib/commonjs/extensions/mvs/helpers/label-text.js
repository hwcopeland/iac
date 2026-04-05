"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.textPropsForSelection = textPropsForSelection;
const boundary_helper_1 = require("../../../mol-math/geometry/boundary-helper.js");
const linear_algebra_1 = require("../../../mol-math/linear-algebra.js");
const structure_1 = require("../../../mol-model/structure.js");
const physical_1 = require("../../../mol-theme/size/physical.js");
const array_1 = require("../../../mol-util/array.js");
const element_ranges_1 = require("./element-ranges.js");
const indexing_1 = require("./indexing.js");
const selections_1 = require("./selections.js");
const utils_1 = require("./utils.js");
const tmpVec = (0, linear_algebra_1.Vec3)();
const tmpArray = [];
const boundaryHelper = new boundary_helper_1.BoundaryHelper('98');
const outElements = [];
const outFirstElementIndex = {};
/** Helper for caching element ranges qualifying to a group of annotation rows, per `Unit`. */
class ElementRangesCache {
    constructor(rows) {
        this.rows = rows;
        this.cache = {};
        this.hasOperators = rows.some(row => (0, utils_1.isDefined)(row.instance_id));
    }
    get(unit) {
        var _a;
        var _b;
        const instanceId = unit.conformation.operator.instanceId;
        const key = `${unit.model.id}:${unit.kind}:${this.hasOperators ? instanceId : '*'}`;
        return (_a = (_b = this.cache)[key]) !== null && _a !== void 0 ? _a : (_b[key] = this.compute(unit));
    }
    compute(unit) {
        const instanceId = unit.conformation.operator.instanceId;
        const indices = indexing_1.IndicesAndSortings.get(unit.model);
        switch (unit.kind) {
            case structure_1.Unit.Kind.Atomic:
                return (0, selections_1.getAtomRangesForRows)(this.rows, unit.model, instanceId, indices);
            case structure_1.Unit.Kind.Spheres:
                return (0, selections_1.getSphereRangesForRows)(this.rows, unit.model, instanceId, indices);
            case structure_1.Unit.Kind.Gaussians:
                return (0, selections_1.getGaussianRangesForRows)(this.rows, unit.model, instanceId, indices);
        }
    }
}
/** Approximate number of heavy atoms per protein residue (I got 7.55 from 2e2n) */
const AVG_ATOMS_PER_RESIDUE = 8;
/** Return `TextProps` (position, size, etc.) for a text that is to be bound to a substructure of `structure` defined by union of `rows`.
 * Derives `center` and `depth` from the boundary sphere of the substructure, `scale` from the number of heavy atoms in the substructure. */
function textPropsForSelection(structure, rows, onlyInModel) {
    const loc = structure_1.StructureElement.Location.create(structure);
    const { units } = structure;
    const { type_symbol } = structure_1.StructureProperties.atom;
    tmpArray.length = 0;
    let includedElements = 0;
    let includedHeavyAtoms = 0;
    let group = undefined;
    /** Used for `depth` in case the selection has only 1 element (hence bounding sphere radius is 0) */
    let singularRadius = undefined;
    const elementRangesCache = new ElementRangesCache(rows);
    for (let iUnit = 0, nUnits = units.length; iUnit < nUnits; iUnit++) {
        const unit = units[iUnit];
        if (onlyInModel && unit.model.id !== onlyInModel.id)
            continue;
        const coarseElements = unit.kind === structure_1.Unit.Kind.Spheres ? unit.model.coarseHierarchy.spheres : unit.kind === structure_1.Unit.Kind.Gaussians ? unit.model.coarseHierarchy.gaussians : undefined;
        const ranges = elementRangesCache.get(unit);
        element_ranges_1.ElementRanges.selectElementsInRanges(unit.elements, ranges, outElements, outFirstElementIndex);
        loc.unit = unit;
        for (const iElem of outElements) {
            loc.element = iElem;
            (0, array_1.arrayExtend)(tmpArray, unit.conformation.position(iElem, tmpVec));
            group !== null && group !== void 0 ? group : (group = structure.serialMapping.cumulativeUnitElementCount[iUnit] + outFirstElementIndex.value);
            singularRadius !== null && singularRadius !== void 0 ? singularRadius : (singularRadius = (0, physical_1.getPhysicalRadius)(unit, iElem) * 1.2);
            if (coarseElements) {
                // coarse
                const nResidues = coarseElements.seq_id_end.value(iElem) - coarseElements.seq_id_begin.value(iElem) + 1;
                includedHeavyAtoms += nResidues * AVG_ATOMS_PER_RESIDUE;
            }
            else {
                // atomic
                if (type_symbol(loc) !== 'H')
                    includedHeavyAtoms++;
            }
            includedElements++;
        }
    }
    if (includedElements > 0) {
        const { center, radius } = (includedElements > 1) ? boundarySphere(tmpArray) : { center: linear_algebra_1.Vec3.fromArray((0, linear_algebra_1.Vec3)(), tmpArray, 0), radius: singularRadius };
        const scale = (includedHeavyAtoms || includedElements) ** (1 / 3);
        return { center, depth: radius, scale, group: group };
    }
}
/** Calculate the boundary sphere for a set of points given by their flattened coordinates (`flatCoords.slice(0,3)` is the first point etc.) */
function boundarySphere(flatCoords) {
    const length = flatCoords.length;
    boundaryHelper.reset();
    for (let offset = 0; offset < length; offset += 3) {
        linear_algebra_1.Vec3.fromArray(tmpVec, flatCoords, offset);
        boundaryHelper.includePosition(tmpVec);
    }
    boundaryHelper.finishedIncludeStep();
    for (let offset = 0; offset < length; offset += 3) {
        linear_algebra_1.Vec3.fromArray(tmpVec, flatCoords, offset);
        boundaryHelper.radiusPosition(tmpVec);
    }
    return boundaryHelper.getSphere();
}
