/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { BoundaryHelper } from '../../../mol-math/geometry/boundary-helper.js';
import { Vec3 } from '../../../mol-math/linear-algebra.js';
import { StructureElement, StructureProperties, Unit } from '../../../mol-model/structure.js';
import { getPhysicalRadius } from '../../../mol-theme/size/physical.js';
import { arrayExtend } from '../../../mol-util/array.js';
import { ElementRanges } from './element-ranges.js';
import { IndicesAndSortings } from './indexing.js';
import { getAtomRangesForRows, getGaussianRangesForRows, getSphereRangesForRows } from './selections.js';
import { isDefined } from './utils.js';
const tmpVec = Vec3();
const tmpArray = [];
const boundaryHelper = new BoundaryHelper('98');
const outElements = [];
const outFirstElementIndex = {};
/** Helper for caching element ranges qualifying to a group of annotation rows, per `Unit`. */
class ElementRangesCache {
    constructor(rows) {
        this.rows = rows;
        this.cache = {};
        this.hasOperators = rows.some(row => isDefined(row.instance_id));
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
        const indices = IndicesAndSortings.get(unit.model);
        switch (unit.kind) {
            case Unit.Kind.Atomic:
                return getAtomRangesForRows(this.rows, unit.model, instanceId, indices);
            case Unit.Kind.Spheres:
                return getSphereRangesForRows(this.rows, unit.model, instanceId, indices);
            case Unit.Kind.Gaussians:
                return getGaussianRangesForRows(this.rows, unit.model, instanceId, indices);
        }
    }
}
/** Approximate number of heavy atoms per protein residue (I got 7.55 from 2e2n) */
const AVG_ATOMS_PER_RESIDUE = 8;
/** Return `TextProps` (position, size, etc.) for a text that is to be bound to a substructure of `structure` defined by union of `rows`.
 * Derives `center` and `depth` from the boundary sphere of the substructure, `scale` from the number of heavy atoms in the substructure. */
export function textPropsForSelection(structure, rows, onlyInModel) {
    const loc = StructureElement.Location.create(structure);
    const { units } = structure;
    const { type_symbol } = StructureProperties.atom;
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
        const coarseElements = unit.kind === Unit.Kind.Spheres ? unit.model.coarseHierarchy.spheres : unit.kind === Unit.Kind.Gaussians ? unit.model.coarseHierarchy.gaussians : undefined;
        const ranges = elementRangesCache.get(unit);
        ElementRanges.selectElementsInRanges(unit.elements, ranges, outElements, outFirstElementIndex);
        loc.unit = unit;
        for (const iElem of outElements) {
            loc.element = iElem;
            arrayExtend(tmpArray, unit.conformation.position(iElem, tmpVec));
            group !== null && group !== void 0 ? group : (group = structure.serialMapping.cumulativeUnitElementCount[iUnit] + outFirstElementIndex.value);
            singularRadius !== null && singularRadius !== void 0 ? singularRadius : (singularRadius = getPhysicalRadius(unit, iElem) * 1.2);
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
        const { center, radius } = (includedElements > 1) ? boundarySphere(tmpArray) : { center: Vec3.fromArray(Vec3(), tmpArray, 0), radius: singularRadius };
        const scale = (includedHeavyAtoms || includedElements) ** (1 / 3);
        return { center, depth: radius, scale, group: group };
    }
}
/** Calculate the boundary sphere for a set of points given by their flattened coordinates (`flatCoords.slice(0,3)` is the first point etc.) */
function boundarySphere(flatCoords) {
    const length = flatCoords.length;
    boundaryHelper.reset();
    for (let offset = 0; offset < length; offset += 3) {
        Vec3.fromArray(tmpVec, flatCoords, offset);
        boundaryHelper.includePosition(tmpVec);
    }
    boundaryHelper.finishedIncludeStep();
    for (let offset = 0; offset < length; offset += 3) {
        Vec3.fromArray(tmpVec, flatCoords, offset);
        boundaryHelper.radiusPosition(tmpVec);
    }
    return boundaryHelper.getSphere();
}
