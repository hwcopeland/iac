/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Mat4 } from '../../mol-math/linear-algebra.js';
import { QueryContext, StructureSelection, StructureElement } from '../../mol-model/structure.js';
import { superpose, alignAndSuperpose } from '../../mol-model/structure/structure/util/superposition.js';
import { tmAlign } from '../../mol-model/structure/structure/util/tm-align.js';
import { MolScriptBuilder as MS } from '../../mol-script/language/builder.js';
import { compile } from '../../mol-script/runtime/query/compiler.js';
import { StateTransforms } from '../../mol-plugin-state/transforms.js';
import { Asset } from '../../mol-util/assets.js';
export function buildStaticSuperposition(plugin, src) {
    return plugin.dataTransaction(async () => {
        for (const s of src) {
            const { structure } = await loadStructure(plugin, `https://www.ebi.ac.uk/pdbe/static/entry/${s.pdbId}_updated.cif`, 'mmcif');
            await transform(plugin, structure, s.matrix);
            const chain = await plugin.builders.structure.tryCreateComponentFromExpression(structure, chainSelection(s.auth_asym_id), `Chain ${s.auth_asym_id}`);
            if (chain)
                await plugin.builders.structure.representation.addRepresentation(chain, { type: 'cartoon' });
        }
    });
}
export const StaticSuperpositionTestData = [
    {
        pdbId: '1aj5', auth_asym_id: 'A', matrix: Mat4.identity()
    },
    {
        pdbId: '1df0', auth_asym_id: 'B', matrix: Mat4.ofRows([
            [0.406, 0.879, 0.248, -200.633],
            [0.693, -0.473, 0.544, 73.403],
            [0.596, -0.049, -0.802, -14.209],
            [0, 0, 0, 1]
        ])
    },
    {
        pdbId: '1dvi', auth_asym_id: 'A', matrix: Mat4.ofRows([
            [-0.053, -0.077, 0.996, -45.633],
            [-0.312, 0.949, 0.057, -12.255],
            [-0.949, -0.307, -0.074, 53.562],
            [0, 0, 0, 1]
        ])
    }
];
export function dynamicSuperpositionTest(plugin, src, comp_id) {
    return plugin.dataTransaction(async () => {
        for (const s of src) {
            await loadStructure(plugin, `https://www.ebi.ac.uk/pdbe/static/entry/${s}_updated.cif`, 'mmcif');
        }
        const pivot = MS.struct.filter.first([
            MS.struct.generator.atomGroups({
                'residue-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.label_comp_id(), comp_id]),
                'group-by': MS.struct.atomProperty.macromolecular.residueKey()
            })
        ]);
        const rest = MS.struct.modifier.exceptBy({
            0: MS.struct.modifier.includeSurroundings({
                0: pivot,
                radius: 5
            }),
            by: pivot
        });
        const query = compile(pivot);
        const xs = plugin.managers.structure.hierarchy.current.structures;
        const selections = xs.map(s => StructureSelection.toLociWithCurrentUnits(query(new QueryContext(s.cell.obj.data))));
        const transforms = superpose(selections);
        await siteVisual(plugin, xs[0].cell, pivot, rest);
        for (let i = 1; i < selections.length; i++) {
            await transform(plugin, xs[i].cell, transforms[i - 1].bTransform);
            await siteVisual(plugin, xs[i].cell, pivot, rest);
        }
    });
}
async function siteVisual(plugin, s, pivot, rest) {
    const center = await plugin.builders.structure.tryCreateComponentFromExpression(s, pivot, 'pivot');
    if (center)
        await plugin.builders.structure.representation.addRepresentation(center, { type: 'ball-and-stick', color: 'residue-name' });
    const surr = await plugin.builders.structure.tryCreateComponentFromExpression(s, rest, 'rest');
    if (surr)
        await plugin.builders.structure.representation.addRepresentation(surr, { type: 'ball-and-stick', color: 'uniform', size: 'uniform', sizeParams: { value: 0.33 } });
}
async function loadStructure(plugin, url, format, assemblyId) {
    const data = await plugin.builders.data.download({ url: Asset.Url(url) });
    const trajectory = await plugin.builders.structure.parseTrajectory(data, format);
    const model = await plugin.builders.structure.createModel(trajectory);
    const structure = await plugin.builders.structure.createStructure(model, assemblyId ? { name: 'assembly', params: { id: assemblyId } } : void 0);
    return { data, trajectory, model, structure };
}
function chainSelection(auth_asym_id) {
    return MS.struct.generator.atomGroups({
        'chain-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), auth_asym_id])
    });
}
function transform(plugin, s, matrix) {
    const b = plugin.state.data.build().to(s)
        .insert(StateTransforms.Model.TransformStructureConformation, { transform: { name: 'matrix', params: { data: matrix, transpose: false } } });
    return plugin.runTask(plugin.state.data.updateTree(b));
}
/**
 * TM-align superposition: aligns two structures using TM-align algorithm
 * @param plugin - Mol* plugin context
 * @param pdbId1 - PDB ID of first structure (reference)
 * @param chain1 - Chain ID of first structure
 * @param pdbId2 - PDB ID of second structure (mobile)
 * @param chain2 - Chain ID of second structure
 * @param color1 - Optional color for first structure (hex, default blue)
 * @param color2 - Optional color for second structure (hex, default red)
 */
export async function tmAlignStructures(plugin, pdbId1, chain1, pdbId2, chain2, color1 = 0x3498db, color2 = 0xe74c3c) {
    var _a, _b, _c, _d;
    await plugin.clear();
    const url1 = `https://files.rcsb.org/download/${pdbId1}.pdb`;
    const url2 = `https://files.rcsb.org/download/${pdbId2}.pdb`;
    const label1 = `${pdbId1} Chain ${chain1}`;
    const label2 = `${pdbId2} Chain ${chain2}`;
    // Load structures
    const struct1 = await loadStructure(plugin, url1, 'pdb');
    const struct2 = await loadStructure(plugin, url2, 'pdb');
    // Build query for C-alpha atoms from specified chains
    const caQuery1 = compile(MS.struct.generator.atomGroups({
        'chain-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), chain1]),
        'atom-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.label_atom_id(), 'CA'])
    }));
    const caQuery2 = compile(MS.struct.generator.atomGroups({
        'chain-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), chain2]),
        'atom-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.label_atom_id(), 'CA'])
    }));
    const structure1Data = (_b = (_a = struct1.structure.cell) === null || _a === void 0 ? void 0 : _a.obj) === null || _b === void 0 ? void 0 : _b.data;
    const structure2Data = (_d = (_c = struct2.structure.cell) === null || _c === void 0 ? void 0 : _c.obj) === null || _d === void 0 ? void 0 : _d.data;
    if (!structure1Data || !structure2Data) {
        console.error('Failed to load structures');
        return undefined;
    }
    const sel1 = StructureSelection.toLociWithCurrentUnits(caQuery1(new QueryContext(structure1Data)));
    const sel2 = StructureSelection.toLociWithCurrentUnits(caQuery2(new QueryContext(structure2Data)));
    const loci1 = StructureElement.Loci.is(sel1) ? sel1 : StructureElement.Loci.none(structure1Data);
    const loci2 = StructureElement.Loci.is(sel2) ? sel2 : StructureElement.Loci.none(structure2Data);
    if (StructureElement.Loci.size(loci1) === 0 || StructureElement.Loci.size(loci2) === 0) {
        console.error('Empty selection - cannot run TM-align');
        // Still show the structures without alignment
        await addChainRepresentation(plugin, struct1.structure, chain1, label1, color1);
        await addChainRepresentation(plugin, struct2.structure, chain2, label2, color2);
        return undefined;
    }
    // Run TM-align
    const result = tmAlign(loci1, loci2);
    console.log('TM-score (structure 1):', result.tmScoreA.toFixed(5));
    console.log('TM-score (structure 2):', result.tmScoreB.toFixed(5));
    console.log('RMSD:', result.rmsd.toFixed(2), 'A');
    console.log('Aligned residues:', result.alignedLength);
    // Apply the transformation to superimpose structure 2 onto structure 1
    await transform(plugin, struct2.structure, result.bTransform);
    // Add cartoon representations
    await addChainRepresentation(plugin, struct1.structure, chain1, label1, color1);
    await addChainRepresentation(plugin, struct2.structure, chain2, label2, color2);
    return {
        tmScoreA: result.tmScoreA,
        tmScoreB: result.tmScoreB,
        rmsd: result.rmsd,
        alignedLength: result.alignedLength
    };
}
async function addChainRepresentation(plugin, structure, chain, label, color) {
    const component = await plugin.builders.structure.tryCreateComponentFromExpression(structure, chainSelection(chain), label);
    if (component) {
        await plugin.builders.structure.representation.addRepresentation(component, {
            type: 'cartoon',
            color: 'uniform',
            colorParams: { value: color }
        });
    }
}
/**
 * Load and display two structures without any alignment
 * @param plugin - Mol* plugin context
 * @param pdbId1 - PDB ID of first structure
 * @param chain1 - Chain ID of first structure
 * @param pdbId2 - PDB ID of second structure
 * @param chain2 - Chain ID of second structure
 * @param color1 - Optional color for first structure (hex, default blue)
 * @param color2 - Optional color for second structure (hex, default red)
 */
export async function loadStructuresNoAlignment(plugin, pdbId1, chain1, pdbId2, chain2, color1 = 0x3498db, color2 = 0xe74c3c) {
    await plugin.clear();
    const url1 = `https://files.rcsb.org/download/${pdbId1}.pdb`;
    const url2 = `https://files.rcsb.org/download/${pdbId2}.pdb`;
    const label1 = `${pdbId1} Chain ${chain1}`;
    const label2 = `${pdbId2} Chain ${chain2}`;
    const struct1 = await loadStructure(plugin, url1, 'pdb');
    const struct2 = await loadStructure(plugin, url2, 'pdb');
    await addChainRepresentation(plugin, struct1.structure, chain1, label1, color1);
    await addChainRepresentation(plugin, struct2.structure, chain2, label2, color2);
    console.log('Loaded structures - NO ALIGNMENT');
}
/**
 * Sequence-based superposition: aligns two structures using sequence alignment + RMSD minimization
 * @param plugin - Mol* plugin context
 * @param pdbId1 - PDB ID of first structure (reference)
 * @param chain1 - Chain ID of first structure
 * @param pdbId2 - PDB ID of second structure (mobile)
 * @param chain2 - Chain ID of second structure
 * @param color1 - Optional color for first structure (hex, default blue)
 * @param color2 - Optional color for second structure (hex, default red)
 */
export async function sequenceAlignStructures(plugin, pdbId1, chain1, pdbId2, chain2, color1 = 0x3498db, color2 = 0xe74c3c) {
    var _a, _b, _c, _d;
    await plugin.clear();
    const url1 = `https://files.rcsb.org/download/${pdbId1}.pdb`;
    const url2 = `https://files.rcsb.org/download/${pdbId2}.pdb`;
    const label1 = `${pdbId1} Chain ${chain1}`;
    const label2 = `${pdbId2} Chain ${chain2}`;
    const struct1 = await loadStructure(plugin, url1, 'pdb');
    const struct2 = await loadStructure(plugin, url2, 'pdb');
    // Build queries for C-alpha atoms from specified chains
    const caQuery1 = compile(MS.struct.generator.atomGroups({
        'chain-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), chain1]),
        'atom-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.label_atom_id(), 'CA'])
    }));
    const caQuery2 = compile(MS.struct.generator.atomGroups({
        'chain-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.auth_asym_id(), chain2]),
        'atom-test': MS.core.rel.eq([MS.struct.atomProperty.macromolecular.label_atom_id(), 'CA'])
    }));
    const structure1Data = (_b = (_a = struct1.structure.cell) === null || _a === void 0 ? void 0 : _a.obj) === null || _b === void 0 ? void 0 : _b.data;
    const structure2Data = (_d = (_c = struct2.structure.cell) === null || _c === void 0 ? void 0 : _c.obj) === null || _d === void 0 ? void 0 : _d.data;
    if (!structure1Data || !structure2Data) {
        console.error('Failed to load structures');
        return { rmsd: 0 };
    }
    const sel1 = StructureSelection.toLociWithCurrentUnits(caQuery1(new QueryContext(structure1Data)));
    const sel2 = StructureSelection.toLociWithCurrentUnits(caQuery2(new QueryContext(structure2Data)));
    // Run sequence alignment + superposition
    const transforms = alignAndSuperpose([sel1, sel2]);
    // Apply the transformation to superimpose structure 2 onto structure 1
    await transform(plugin, struct2.structure, transforms[0].bTransform);
    // Add cartoon representations
    await addChainRepresentation(plugin, struct1.structure, chain1, label1, color1);
    await addChainRepresentation(plugin, struct2.structure, chain2, label2, color2);
    console.log('RMSD:', transforms[0].rmsd.toFixed(2), 'A');
    return { rmsd: transforms[0].rmsd };
}
