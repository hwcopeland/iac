"use strict";
/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.getAtomRangesForRow = getAtomRangesForRow;
exports.getAtomRangesForRows = getAtomRangesForRows;
exports.atomQualifies = atomQualifies;
exports.getSphereRangesForRow = getSphereRangesForRow;
exports.getSphereRangesForRows = getSphereRangesForRows;
exports.getGaussianRangesForRow = getGaussianRangesForRow;
exports.getGaussianRangesForRows = getGaussianRangesForRows;
exports.getCoarseElementRangesForRow = getCoarseElementRangesForRow;
exports.rowToExpression = rowToExpression;
exports.rowsToExpression = rowsToExpression;
const db_1 = require("../../../mol-data/db.js");
const int_1 = require("../../../mol-data/int.js");
const structure_1 = require("../../../mol-model/structure.js");
const array_1 = require("../../../mol-util/array.js");
const element_ranges_1 = require("./element-ranges.js");
const indexing_1 = require("./indexing.js");
const utils_1 = require("./utils.js");
const EmptyArray = [];
// ATOMIC SELECTIONS
/** Return atom ranges in `model` which satisfy criteria given by `row` */
function getAtomRangesForRow(row, model, instanceId, indices) {
    if (!indices.atomic)
        return undefined;
    if ((0, utils_1.isDefined)(row.instance_id) && row.instance_id !== instanceId)
        return undefined;
    const atomicIndices = indices.atomic;
    const h = model.atomicHierarchy;
    const nAtoms = h.atoms._rowCount;
    if (nAtoms === 0)
        return undefined;
    const hasAtomIds = (0, utils_1.isAnyDefined)(row.atom_id, row.atom_index);
    const hasAtomFilter = (0, utils_1.isAnyDefined)(row.label_atom_id, row.auth_atom_id, row.type_symbol)
        || (0, utils_1.isDefined)(row.label_comp_id) && !atomicIndices.residuesByLabelCompIdIsPure
        || (0, utils_1.isDefined)(row.auth_comp_id) && !atomicIndices.residuesByAuthCompIdIsPure;
    const hasResidueFilter = (0, utils_1.isAnyDefined)(row.label_seq_id, row.auth_seq_id, row.pdbx_PDB_ins_code, row.beg_label_seq_id, row.end_label_seq_id, row.beg_auth_seq_id, row.end_auth_seq_id, row.label_comp_id, row.auth_comp_id, row.residue_index);
    const hasChainFilter = (0, utils_1.isAnyDefined)(row.label_asym_id, row.auth_asym_id, row.label_entity_id);
    if (hasAtomIds) {
        const theAtom = getTheAtomForRow(model, row, atomicIndices);
        return theAtom !== undefined ? element_ranges_1.ElementRanges.single(theAtom, theAtom + 1) : undefined;
    }
    if (!hasChainFilter && !hasResidueFilter && !hasAtomFilter) {
        return element_ranges_1.ElementRanges.single(0, nAtoms);
    }
    const qualifyingChains = getQualifyingChains(model, row, atomicIndices);
    if (!hasResidueFilter && !hasAtomFilter) {
        const chainOffsets = h.chainAtomSegments.offsets;
        const ranges = element_ranges_1.ElementRanges.empty();
        for (const iChain of qualifyingChains) {
            element_ranges_1.ElementRanges.add(ranges, chainOffsets[iChain], chainOffsets[iChain + 1]);
        }
        return ranges;
    }
    const qualifyingResidues = getQualifyingResidues(model, row, atomicIndices, qualifyingChains);
    if (!hasAtomFilter) {
        const residueOffsets = h.residueAtomSegments.offsets;
        const ranges = element_ranges_1.ElementRanges.empty();
        for (const iRes of qualifyingResidues) {
            element_ranges_1.ElementRanges.add(ranges, residueOffsets[iRes], residueOffsets[iRes + 1]);
        }
        return ranges;
    }
    const qualifyingAtoms = getQualifyingAtoms(model, row, atomicIndices, qualifyingResidues);
    const ranges = element_ranges_1.ElementRanges.empty();
    for (const iAtom of qualifyingAtoms) {
        element_ranges_1.ElementRanges.add(ranges, iAtom, iAtom + 1);
    }
    return ranges;
}
/** Return atom ranges in `model` which satisfy criteria given by any of `rows` (atoms that satisfy more rows are still included only once) */
function getAtomRangesForRows(rows, model, instanceId, indices) {
    return element_ranges_1.ElementRanges.union(rows.map(row => getAtomRangesForRow(row, model, instanceId, indices)));
}
/** Return an array of chain indexes which satisfy criteria given by `row` */
function getQualifyingChains(model, row, indices) {
    var _a, _b, _c;
    const { auth_asym_id, label_entity_id, _rowCount: nChains } = model.atomicHierarchy.chains;
    let result = undefined;
    if ((0, utils_1.isDefined)(row.label_asym_id)) {
        result = (_a = indices.chainsByLabelAsymId.get(row.label_asym_id)) !== null && _a !== void 0 ? _a : EmptyArray;
    }
    if ((0, utils_1.isDefined)(row.auth_asym_id)) {
        if (result) {
            result = result.filter(i => auth_asym_id.value(i) === row.auth_asym_id);
        }
        else {
            result = (_b = indices.chainsByAuthAsymId.get(row.auth_asym_id)) !== null && _b !== void 0 ? _b : EmptyArray;
        }
    }
    if ((0, utils_1.isDefined)(row.label_entity_id)) {
        if (result) {
            result = result.filter(i => label_entity_id.value(i) === row.label_entity_id);
        }
        else {
            result = (_c = indices.chainsByLabelEntityId.get(row.label_entity_id)) !== null && _c !== void 0 ? _c : EmptyArray;
        }
    }
    result !== null && result !== void 0 ? result : (result = (0, array_1.range)(nChains));
    return result;
}
/** Return an array of residue indexes which satisfy criteria given by `row` */
function getQualifyingResidues(model, row, indices, fromChains) {
    var _a, _b, _c;
    const { label_seq_id, auth_seq_id, pdbx_PDB_ins_code } = model.atomicHierarchy.residues;
    const { label_comp_id, auth_comp_id } = model.atomicHierarchy.atoms;
    const { residueAtomSegments, chainAtomSegments, residueSourceIndex } = model.atomicHierarchy;
    const { Present } = db_1.Column.ValueKind;
    const result = [];
    for (const iChain of fromChains) {
        let residuesHere = undefined;
        if ((0, utils_1.isDefined)(row.label_seq_id)) {
            const sorting = indices.residuesSortedByLabelSeqId.get(iChain);
            residuesHere = indexing_1.Sorting.getKeysWithValue(sorting, row.label_seq_id);
        }
        if ((0, utils_1.isDefined)(row.auth_seq_id)) {
            if (residuesHere) {
                residuesHere = residuesHere.filter(i => auth_seq_id.valueKind(i) === Present && auth_seq_id.value(i) === row.auth_seq_id);
            }
            else {
                const sorting = indices.residuesSortedByAuthSeqId.get(iChain);
                residuesHere = indexing_1.Sorting.getKeysWithValue(sorting, row.auth_seq_id);
            }
        }
        if ((0, utils_1.isDefined)(row.residue_index)) {
            if (residuesHere) {
                residuesHere = residuesHere.filter(i => residueSourceIndex.value(i) === row.residue_index);
            }
            else {
                const sorting = indices.residuesSortedBySourceIndex.get(iChain);
                residuesHere = indexing_1.Sorting.getKeysWithValue(sorting, row.residue_index);
            }
        }
        if ((0, utils_1.isDefined)(row.pdbx_PDB_ins_code)) {
            if (residuesHere) {
                residuesHere = residuesHere.filter(i => pdbx_PDB_ins_code.value(i) === row.pdbx_PDB_ins_code);
            }
            else {
                residuesHere = (_a = indices.residuesByInsCode.get(iChain).get(row.pdbx_PDB_ins_code)) !== null && _a !== void 0 ? _a : EmptyArray;
            }
        }
        if ((0, utils_1.isDefined)(row.beg_label_seq_id) || (0, utils_1.isDefined)(row.end_label_seq_id)) {
            if (residuesHere) {
                if ((0, utils_1.isDefined)(row.beg_label_seq_id)) {
                    residuesHere = residuesHere.filter(i => label_seq_id.valueKind(i) === Present && label_seq_id.value(i) >= row.beg_label_seq_id);
                }
                if ((0, utils_1.isDefined)(row.end_label_seq_id)) {
                    residuesHere = residuesHere.filter(i => label_seq_id.valueKind(i) === Present && label_seq_id.value(i) <= row.end_label_seq_id);
                }
            }
            else {
                const sorting = indices.residuesSortedByLabelSeqId.get(iChain);
                residuesHere = indexing_1.Sorting.getKeysWithValueInRange(sorting, row.beg_label_seq_id, row.end_label_seq_id);
            }
        }
        if ((0, utils_1.isDefined)(row.beg_auth_seq_id) || (0, utils_1.isDefined)(row.end_auth_seq_id)) {
            if (residuesHere) {
                if ((0, utils_1.isDefined)(row.beg_auth_seq_id)) {
                    residuesHere = residuesHere.filter(i => auth_seq_id.valueKind(i) === Present && auth_seq_id.value(i) >= row.beg_auth_seq_id);
                }
                if ((0, utils_1.isDefined)(row.end_auth_seq_id)) {
                    residuesHere = residuesHere.filter(i => auth_seq_id.valueKind(i) === Present && auth_seq_id.value(i) <= row.end_auth_seq_id);
                }
            }
            else {
                const sorting = indices.residuesSortedByAuthSeqId.get(iChain);
                residuesHere = indexing_1.Sorting.getKeysWithValueInRange(sorting, row.beg_auth_seq_id, row.end_auth_seq_id);
            }
        }
        if ((0, utils_1.isDefined)(row.label_comp_id)) {
            if (residuesHere) {
                if (indices.residuesByLabelCompIdIsPure) {
                    residuesHere = residuesHere.filter(i => label_comp_id.value(residueAtomSegments.offsets[i]) === row.label_comp_id);
                }
                else {
                    residuesHere = residuesHere.filter(i => {
                        for (let iAtom = residueAtomSegments.offsets[i], stop = residueAtomSegments.offsets[i + 1]; iAtom < stop; iAtom++) {
                            if (label_comp_id.value(iAtom) === row.label_comp_id)
                                return true;
                        }
                    });
                }
            }
            else {
                residuesHere = (_b = indices.residuesByLabelCompId.get(iChain).get(row.label_comp_id)) !== null && _b !== void 0 ? _b : EmptyArray;
            }
        }
        if ((0, utils_1.isDefined)(row.auth_comp_id)) {
            if (residuesHere) {
                if (indices.residuesByAuthCompIdIsPure) {
                    residuesHere = residuesHere.filter(i => auth_comp_id.value(residueAtomSegments.offsets[i]) === row.auth_comp_id);
                }
                else {
                    residuesHere = residuesHere.filter(i => {
                        for (let iAtom = residueAtomSegments.offsets[i], stop = residueAtomSegments.offsets[i + 1]; iAtom < stop; iAtom++) {
                            if (auth_comp_id.value(iAtom) === row.auth_comp_id)
                                return true;
                        }
                    });
                }
            }
            else {
                residuesHere = (_c = indices.residuesByAuthCompId.get(iChain).get(row.auth_comp_id)) !== null && _c !== void 0 ? _c : EmptyArray;
            }
        }
        if (!residuesHere) {
            const firstResidueForChain = residueAtomSegments.index[chainAtomSegments.offsets[iChain]];
            const firstResidueAfterChain = residueAtomSegments.index[chainAtomSegments.offsets[iChain + 1] - 1] + 1;
            residuesHere = (0, array_1.range)(firstResidueForChain, firstResidueAfterChain);
        }
        (0, array_1.arrayExtend)(result, residuesHere);
    }
    (0, array_1.sortIfNeeded)(result, (a, b) => a - b);
    return result;
}
/** Return an array of atom indexes which satisfy criteria given by `row` */
function getQualifyingAtoms(model, row, indices, fromResidues) {
    const { label_atom_id, auth_atom_id, type_symbol, label_comp_id, auth_comp_id } = model.atomicHierarchy.atoms;
    const residueAtomSegments_offsets = model.atomicHierarchy.residueAtomSegments.offsets;
    const result = [];
    for (const iRes of fromResidues) {
        const atomIdcs = (0, array_1.range)(residueAtomSegments_offsets[iRes], residueAtomSegments_offsets[iRes + 1]);
        if ((0, utils_1.isDefined)(row.label_atom_id)) {
            (0, array_1.filterInPlace)(atomIdcs, iAtom => label_atom_id.value(iAtom) === row.label_atom_id);
        }
        if ((0, utils_1.isDefined)(row.auth_atom_id)) {
            (0, array_1.filterInPlace)(atomIdcs, iAtom => auth_atom_id.value(iAtom) === row.auth_atom_id);
        }
        if ((0, utils_1.isDefined)(row.type_symbol)) {
            (0, array_1.filterInPlace)(atomIdcs, iAtom => { var _a; return type_symbol.value(iAtom) === ((_a = row.type_symbol) === null || _a === void 0 ? void 0 : _a.toUpperCase()); });
        }
        if ((0, utils_1.isDefined)(row.label_comp_id) && !indices.residuesByLabelCompIdIsPure) {
            (0, array_1.filterInPlace)(atomIdcs, iAtom => label_comp_id.value(iAtom) === row.label_comp_id);
        }
        if ((0, utils_1.isDefined)(row.auth_comp_id) && !indices.residuesByAuthCompIdIsPure) {
            (0, array_1.filterInPlace)(atomIdcs, iAtom => auth_comp_id.value(iAtom) === row.auth_comp_id);
        }
        (0, array_1.arrayExtend)(result, atomIdcs);
    }
    return result;
}
/** Return index of atom in `model` which satistfies criteria given by `row`, if any.
 * Only works when `row.atom_id` and/or `row.atom_index` is defined (otherwise use `getAtomRangesForRow`). */
function getTheAtomForRow(model, row, indices) {
    let iAtom = undefined;
    if (!(0, utils_1.isDefined)(row.atom_id) && !(0, utils_1.isDefined)(row.atom_index))
        throw new Error('ArgumentError: at least one of row.atom_id, row.atom_index must be defined.');
    if ((0, utils_1.isDefined)(row.atom_id) && (0, utils_1.isDefined)(row.atom_index)) {
        const a1 = indices.atomsById.get(row.atom_id);
        const a2 = indices.atomsBySourceIndex.get(row.atom_index);
        if (a1 !== a2)
            return undefined;
        iAtom = a1;
    }
    if ((0, utils_1.isDefined)(row.atom_id)) {
        iAtom = indices.atomsById.get(row.atom_id);
    }
    if ((0, utils_1.isDefined)(row.atom_index)) {
        iAtom = indices.atomsBySourceIndex.get(row.atom_index);
    }
    if (iAtom === undefined)
        return undefined;
    if (!atomQualifies(model, iAtom, row))
        return undefined;
    return iAtom;
}
/** Return true if `iAtom`-th atom in `model` satisfies all selection criteria given by `row`. */
function atomQualifies(model, iAtom, row) {
    var _a;
    const h = model.atomicHierarchy;
    const iChain = h.chainAtomSegments.index[iAtom];
    const label_asym_id = h.chains.label_asym_id.value(iChain);
    const auth_asym_id = h.chains.auth_asym_id.value(iChain);
    const label_entity_id = h.chains.label_entity_id.value(iChain);
    if (!matches(row.label_asym_id, label_asym_id))
        return false;
    if (!matches(row.auth_asym_id, auth_asym_id))
        return false;
    if (!matches(row.label_entity_id, label_entity_id))
        return false;
    const iRes = h.residueAtomSegments.index[iAtom];
    const label_seq_id = (h.residues.label_seq_id.valueKind(iRes) === db_1.Column.ValueKind.Present) ? h.residues.label_seq_id.value(iRes) : undefined;
    const auth_seq_id = (h.residues.auth_seq_id.valueKind(iRes) === db_1.Column.ValueKind.Present) ? h.residues.auth_seq_id.value(iRes) : undefined;
    const pdbx_PDB_ins_code = h.residues.pdbx_PDB_ins_code.value(iRes);
    const residue_index = h.residueSourceIndex.value(iRes);
    if (!matches(row.label_seq_id, label_seq_id))
        return false;
    if (!matches(row.auth_seq_id, auth_seq_id))
        return false;
    if (!matches(row.pdbx_PDB_ins_code, pdbx_PDB_ins_code))
        return false;
    if (!matchesRange(row.beg_label_seq_id, row.end_label_seq_id, label_seq_id))
        return false;
    if (!matchesRange(row.beg_auth_seq_id, row.end_auth_seq_id, auth_seq_id))
        return false;
    if (!matches(row.residue_index, residue_index))
        return false;
    const label_comp_id = h.atoms.label_comp_id.value(iAtom);
    const auth_comp_id = h.atoms.auth_comp_id.value(iAtom);
    const label_atom_id = h.atoms.label_atom_id.value(iAtom);
    const auth_atom_id = h.atoms.auth_atom_id.value(iAtom);
    const type_symbol = h.atoms.type_symbol.value(iAtom);
    const atom_id = model.atomicConformation.atomId.value(iAtom);
    const atom_index = h.atomSourceIndex.value(iAtom);
    if (!matches(row.label_comp_id, label_comp_id))
        return false;
    if (!matches(row.auth_comp_id, auth_comp_id))
        return false;
    if (!matches(row.label_atom_id, label_atom_id))
        return false;
    if (!matches(row.auth_atom_id, auth_atom_id))
        return false;
    if (!matches((_a = row.type_symbol) === null || _a === void 0 ? void 0 : _a.toUpperCase(), type_symbol))
        return false;
    if (!matches(row.atom_id, atom_id))
        return false;
    if (!matches(row.atom_index, atom_index))
        return false;
    return true;
}
/** Return true if `value` equals `requiredValue` or if `requiredValue` if not defined.  */
function matches(requiredValue, value) {
    return !(0, utils_1.isDefined)(requiredValue) || value === requiredValue;
}
/** Return true if `requiredMin <= value <= requiredMax`.
 * Undefined `requiredMin` behaves like negative infinity.
 * Undefined `requiredMax` behaves like positive infinity. */
function matchesRange(requiredMin, requiredMax, value) {
    if ((0, utils_1.isDefined)(requiredMin) && (!(0, utils_1.isDefined)(value) || value < requiredMin))
        return false;
    if ((0, utils_1.isDefined)(requiredMax) && (!(0, utils_1.isDefined)(value) || value > requiredMax))
        return false;
    return true;
}
// COARSE SELECTIONS
/** Return sphere ranges in `model` which satisfy criteria given by `row` */
function getSphereRangesForRow(row, model, instanceId, indices) {
    if (!indices.spheres)
        return undefined;
    if ((0, utils_1.isDefined)(row.instance_id) && row.instance_id !== instanceId)
        return undefined;
    return getCoarseElementRangesForRow(row, model.coarseHierarchy.spheres, indices.spheres);
}
/** Return sphere ranges in `model` which satisfy criteria given by any of `rows` (spheres that satisfy more rows are still included only once) */
function getSphereRangesForRows(rows, model, instanceId, indices) {
    return element_ranges_1.ElementRanges.union(rows.map(row => getSphereRangesForRow(row, model, instanceId, indices)));
}
/** Return gaussian ranges in `model` which satisfy criteria given by `row` */
function getGaussianRangesForRow(row, model, instanceId, indices) {
    if (!indices.gaussians)
        return undefined;
    if ((0, utils_1.isDefined)(row.instance_id) && row.instance_id !== instanceId)
        return undefined;
    return getCoarseElementRangesForRow(row, model.coarseHierarchy.gaussians, indices.gaussians);
}
/** Return gaussian ranges in `model` which satisfy criteria given by any of `rows` (gaussians that satisfy more rows are still included only once) */
function getGaussianRangesForRows(rows, model, instanceId, indices) {
    return element_ranges_1.ElementRanges.union(rows.map(row => getGaussianRangesForRow(row, model, instanceId, indices)));
}
/** Return ranges of coarse elements (spheres or gaussians) which satisfy criteria given by `row` */
function getCoarseElementRangesForRow(row, coarseElements, indices) {
    const nElements = coarseElements.count;
    if (nElements === 0)
        return undefined;
    const hasResidueFilter = (0, utils_1.isAnyDefined)(row.label_seq_id, row.beg_label_seq_id, row.end_label_seq_id);
    const hasChainFilter = (0, utils_1.isAnyDefined)(row.label_asym_id, row.label_entity_id);
    const hasInvalidFilter = (0, utils_1.isAnyDefined)(row.auth_asym_id, row.auth_seq_id, row.pdbx_PDB_ins_code, row.beg_auth_seq_id, row.end_auth_seq_id, row.label_comp_id, row.auth_comp_id, row.residue_index, row.label_atom_id, row.auth_atom_id, row.type_symbol, row.atom_id, row.atom_index);
    if (hasInvalidFilter) {
        printCoarseSelectorWarning();
        return undefined;
    }
    if (!hasChainFilter && !hasResidueFilter) {
        return element_ranges_1.ElementRanges.single(0, nElements);
    }
    const qualifyingChains = getQualifyingCoarseChains(coarseElements, row, indices);
    if (!hasResidueFilter) {
        const chainOffsets = coarseElements.chainElementSegments.offsets;
        const ranges = element_ranges_1.ElementRanges.empty();
        for (const iChain of qualifyingChains) {
            element_ranges_1.ElementRanges.add(ranges, chainOffsets[iChain], chainOffsets[iChain + 1]);
        }
        return ranges;
    }
    const qualifyingElements = getQualifyingCoarseElements(coarseElements, row, indices, qualifyingChains);
    const ranges = element_ranges_1.ElementRanges.empty();
    for (const iElem of qualifyingElements) {
        element_ranges_1.ElementRanges.add(ranges, iElem, iElem + 1);
    }
    return ranges;
}
/** Return an array of chain indexes which satisfy criteria given by `row` */
function getQualifyingCoarseChains(coarseElements, row, indices) {
    var _a, _b;
    let result = undefined;
    if ((0, utils_1.isDefined)(row.label_asym_id)) {
        result = (_a = indices.chainsByAsymId.get(row.label_asym_id)) !== null && _a !== void 0 ? _a : EmptyArray;
    }
    if ((0, utils_1.isDefined)(row.label_entity_id)) {
        if (result) {
            result = result.filter(iChain => coarseElements.entity_id.value(coarseElements.chainElementSegments.offsets[iChain]) === row.label_entity_id);
        }
        else {
            result = (_b = indices.chainsByEntityId.get(row.label_entity_id)) !== null && _b !== void 0 ? _b : EmptyArray;
        }
    }
    result !== null && result !== void 0 ? result : (result = (0, array_1.range)(coarseElements.chainElementSegments.count));
    return result;
}
/** Return an array of residue indexes which satisfy criteria given by `row` */
function getQualifyingCoarseElements(coarseElements, row, indices, fromChains) {
    var _a, _b, _c, _d;
    const result = [];
    for (const iChain of fromChains) {
        const sorting = indices.elementsSortedBySeqIdBegin.get(iChain);
        const queryStart = Math.max((_a = row.label_seq_id) !== null && _a !== void 0 ? _a : -Infinity, (_b = row.beg_label_seq_id) !== null && _b !== void 0 ? _b : -Infinity);
        const queryEnd = Math.min((_c = row.label_seq_id) !== null && _c !== void 0 ? _c : Infinity, (_d = row.end_label_seq_id) !== null && _d !== void 0 ? _d : Infinity); // inclusive
        const iStart = int_1.SortedArray.findPredecessorIndex(sorting.endUpperBounds, queryStart); // select elements potentially ending >=queryStart (necessary condition)
        const iStop = int_1.SortedArray.findPredecessorIndex(sorting.values, queryEnd + 1); // select elements starting <=queryEnd (necessary and suffient condition) // exclusive
        for (let i = iStart; i < iStop; i++) {
            const iElem = sorting.keys[i];
            if (coarseElements.seq_id_end.value(iElem) >= queryStart) { // rechecking seq_id_end, as the condition was not sufficient
                result.push(iElem);
            }
        }
        // This implementation can yield some elements even when queryStart>queryEnd (e.g. { beg_label_seq_id: 70, end_label_seq_id: 58, label_seq_id: 60 } -> sphere 51-100 qualifies ).
        // This is on purpose, to have the same behavior as MolScript.
    }
    (0, array_1.sortIfNeeded)(result, (a, b) => a - b);
    return result;
}
let coarseSelectorWarningPrinted = false;
function printCoarseSelectorWarning() {
    if (!coarseSelectorWarningPrinted) {
        console.warn('Using unsupported selector fields (auth_asym_id, auth_seq_id, pdbx_PDB_ins_code, beg_auth_seq_id, end_auth_seq_id, label_comp_id, auth_comp_id, residue_index, label_atom_id, auth_atom_id, type_symbol, atom_id, atom_index) on a coarse structure. The resulting selection will be empty.');
        coarseSelectorWarningPrinted = true;
    }
}
// GENERAL
/** Convert an annotation row into a MolScript expression */
function rowToExpression(row) {
    return structure_1.StructureElement.Schema.toExpression(row);
}
/** Convert multiple annotation rows into a MolScript expression.
 * (with union semantics, i.e. an atom qualifies if it qualifies for at least one of the rows) */
function rowsToExpression(rows) {
    return structure_1.StructureElement.Schema.toExpression({
        items: rows
    });
}
