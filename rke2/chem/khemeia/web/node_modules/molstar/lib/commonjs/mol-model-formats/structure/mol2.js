"use strict";
/**
 * Copyright (c) 2020-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Mol2Format = void 0;
exports.trajectoryFromMol2 = trajectoryFromMol2;
const db_1 = require("../../mol-data/db.js");
const types_1 = require("../../mol-model/structure/model/types.js");
const mol_task_1 = require("../../mol-task/index.js");
const parser_1 = require("./basic/parser.js");
const schema_1 = require("./basic/schema.js");
const component_1 = require("./common/component.js");
const entity_1 = require("./common/entity.js");
const index_pair_1 = require("./property/bonds/index-pair.js");
const partial_charge_1 = require("./property/partial-charge.js");
const structure_1 = require("../../mol-model/structure.js");
const util_1 = require("./util.js");
const symmetry_1 = require("./property/symmetry.js");
const geometry_1 = require("../../mol-math/geometry.js");
const linear_algebra_1 = require("../../mol-math/linear-algebra.js");
const util_2 = require("./common/util.js");
async function getModels(mol2, ctx) {
    const models = [];
    for (let i = 0, il = mol2.structures.length; i < il; ++i) {
        const { molecule, atoms, bonds, substructures, crysin } = mol2.structures[i];
        const entityIds = new Array(atoms.count);
        const asymIds = new Array(atoms.count);
        const typeSymbols = new Array(atoms.count);
        const compIds = new Array(atoms.count);
        const seqIds = new Array(atoms.count);
        let hasAtomType = false;
        for (let i = 0; i < atoms.count; ++i) {
            if (atoms.atom_type.value(i).includes('.')) {
                hasAtomType = true;
                break;
            }
        }
        const substByRoot = new Map();
        if (substructures) {
            for (let i = 0; i < substructures.count; ++i) {
                const rootAtom = substructures.root_atom.value(i);
                const chain = substructures.chain.value(i);
                const subType = substructures.sub_type.value(i);
                substByRoot.set(rootAtom, { chain, subType });
            }
        }
        const entityBuilder = new entity_1.EntityBuilder();
        const componentBuilder = new component_1.ComponentBuilder(atoms.subst_id, atoms.atom_name);
        let currentSubst = undefined;
        let currentEntityId = '';
        let currentAsymIndex = 0;
        let currentAsymId = '';
        let prevMoleculeType = types_1.MoleculeType.Unknown;
        let prevSubstId = -1;
        for (let i = 0; i < atoms.count; ++i) {
            const substName = atoms.subst_name.value(i);
            const substId = atoms.subst_id.value(i);
            let compId = substName.replace(/\d+$/, '') || 'MOL';
            const seqIdMatch = substName.match(/(\d+)$/);
            seqIds[i] = seqIdMatch ? parseInt(seqIdMatch[1]) : substId;
            const subst = substByRoot.get(atoms.atom_id.value(i));
            if (subst)
                currentSubst = subst;
            if (currentSubst) {
                if (currentSubst.subType && currentSubst.subType !== 'UNK') {
                    compId = currentSubst.subType;
                }
            }
            typeSymbols[i] = hasAtomType
                ? atoms.atom_type.value(i).split('.')[0].toUpperCase()
                : (0, util_1.guessElementSymbolString)(atoms.atom_name.value(i), compId);
            compIds[i] = compId;
            if (substId !== prevSubstId) {
                const moleculeType = (0, types_1.getMoleculeType)(componentBuilder.add(compId, i).type, compId);
                if (currentSubst === null || currentSubst === void 0 ? void 0 : currentSubst.chain) {
                    currentAsymId = currentSubst.chain;
                }
                else {
                    if (moleculeType !== prevMoleculeType || substId !== prevSubstId + 1) {
                        currentAsymId = (0, util_2.getChainId)(currentAsymIndex);
                        currentAsymIndex += 1;
                    }
                }
                currentEntityId = entityBuilder.getEntityId(compId, moleculeType, currentAsymId);
                prevSubstId = substId;
                prevMoleculeType = moleculeType;
            }
            entityIds[i] = currentEntityId;
            asymIds[i] = currentAsymId;
        }
        const type_symbol = db_1.Column.ofStringArray(typeSymbols);
        const comp_id = db_1.Column.ofStringArray(compIds);
        const entity_id = db_1.Column.ofStringArray(entityIds);
        const asym_id = db_1.Column.ofStringArray(asymIds);
        const atom_id = db_1.Column.asArrayColumn(atoms.atom_name);
        const seq_id = db_1.Column.ofIntArray(seqIds);
        const atom_site = db_1.Table.ofPartialColumns(schema_1.BasicSchema.atom_site, {
            auth_asym_id: asym_id,
            auth_atom_id: atom_id,
            auth_comp_id: comp_id,
            auth_seq_id: seq_id,
            Cartn_x: db_1.Column.asArrayColumn(atoms.x, Float32Array),
            Cartn_y: db_1.Column.asArrayColumn(atoms.y, Float32Array),
            Cartn_z: db_1.Column.asArrayColumn(atoms.z, Float32Array),
            id: db_1.Column.asArrayColumn(atoms.atom_id),
            label_asym_id: asym_id,
            label_atom_id: atom_id,
            label_comp_id: comp_id,
            label_seq_id: atoms.subst_id,
            label_entity_id: entity_id,
            occupancy: db_1.Column.ofConst(1, atoms.count, db_1.Column.Schema.float),
            type_symbol,
            pdbx_PDB_model_num: db_1.Column.ofConst(i, atoms.count, db_1.Column.Schema.int),
        }, atoms.count);
        const basic = (0, schema_1.createBasic)({
            entity: entityBuilder.getEntityTable(),
            chem_comp: componentBuilder.getChemCompTable(),
            atom_site
        });
        const _models = await (0, parser_1.createModels)(basic, Mol2Format.create(mol2), ctx);
        if (_models.frameCount > 0) {
            const indexA = db_1.Column.ofIntArray(db_1.Column.mapToArray(bonds.origin_atom_id, x => x - 1, Int32Array));
            const indexB = db_1.Column.ofIntArray(db_1.Column.mapToArray(bonds.target_atom_id, x => x - 1, Int32Array));
            const key = bonds.bond_id;
            const order = db_1.Column.ofIntArray(db_1.Column.mapToArray(bonds.bond_type, x => {
                switch (x) {
                    case 'ar': // aromatic
                    case 'am': // amide
                    case 'un': // unknown
                        return 1;
                    case 'du': // dummy
                    case 'nc': // not connected
                        return 0;
                    default:
                        return parseInt(x);
                }
            }, Int8Array));
            const flag = db_1.Column.ofIntArray(db_1.Column.mapToArray(bonds.bond_type, x => {
                switch (x) {
                    case 'ar': // aromatic
                    case 'am': // amide
                        return types_1.BondType.Flag.Aromatic | types_1.BondType.Flag.Covalent;
                    case 'du': // dummy
                    case 'nc': // not connected
                        return types_1.BondType.Flag.None;
                    case 'un': // unknown
                    default:
                        return types_1.BondType.Flag.Covalent;
                }
            }, Int8Array));
            const pairBonds = index_pair_1.IndexPairBonds.fromData({ pairs: { key, indexA, indexB, order, flag }, count: atoms.count }, { maxDistance: crysin ? -1 : Infinity });
            const first = _models.representative;
            index_pair_1.IndexPairBonds.Provider.set(first, pairBonds);
            partial_charge_1.AtomPartialCharge.Provider.set(first, {
                data: atoms.charge,
                type: molecule.charge_type
            });
            if (crysin) {
                const symmetry = getSymmetry(crysin);
                if (symmetry)
                    symmetry_1.ModelSymmetry.Provider.set(first, symmetry);
            }
            models.push(first);
        }
    }
    return new structure_1.ArrayTrajectory(models);
}
function getSymmetry(crysin) {
    // TODO handle `crysin.setting`
    if (crysin.setting !== 1)
        return;
    const spaceCell = geometry_1.SpacegroupCell.create(crysin.spaceGroup, linear_algebra_1.Vec3.create(crysin.a, crysin.b, crysin.c), linear_algebra_1.Vec3.scale((0, linear_algebra_1.Vec3)(), linear_algebra_1.Vec3.create(crysin.alpha, crysin.beta, crysin.gamma), Math.PI / 180));
    return {
        spacegroup: geometry_1.Spacegroup.create(spaceCell),
        assemblies: [],
        isNonStandardCrystalFrame: false,
        ncsOperators: []
    };
}
var Mol2Format;
(function (Mol2Format) {
    function is(x) {
        return (x === null || x === void 0 ? void 0 : x.kind) === 'mol2';
    }
    Mol2Format.is = is;
    function create(mol2) {
        return { kind: 'mol2', name: mol2.name, data: mol2 };
    }
    Mol2Format.create = create;
})(Mol2Format || (exports.Mol2Format = Mol2Format = {}));
function trajectoryFromMol2(mol2) {
    return mol_task_1.Task.create('Parse MOL2', ctx => getModels(mol2, ctx));
}
