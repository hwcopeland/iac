/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { MVSTree } from '../mvs/mvs-tree.js';
import { MolstarTree } from './molstar-tree.js';
/** Convert `format` parameter of `parse` node in `MolstarTree`
 * into `format` and `is_binary` parameters in `MolstarTree` */
export declare const ParseFormatMvsToMolstar: {
    mmcif: {
        format: "cif";
        is_binary: false;
    };
    bcif: {
        format: "cif";
        is_binary: true;
    };
    pdb: {
        format: "pdb";
        is_binary: false;
    };
    pdbqt: {
        format: "pdbqt";
        is_binary: false;
    };
    gro: {
        format: "gro";
        is_binary: false;
    };
    xyz: {
        format: "xyz";
        is_binary: false;
    };
    mol: {
        format: "mol";
        is_binary: false;
    };
    sdf: {
        format: "sdf";
        is_binary: false;
    };
    mol2: {
        format: "mol2";
        is_binary: false;
    };
    lammpstrj: {
        format: "lammpstrj";
        is_binary: false;
    };
    xtc: {
        format: "xtc";
        is_binary: true;
    };
    nctraj: {
        format: "nctraj";
        is_binary: true;
    };
    dcd: {
        format: "dcd";
        is_binary: true;
    };
    trr: {
        format: "trr";
        is_binary: true;
    };
    psf: {
        format: "psf";
        is_binary: false;
    };
    prmtop: {
        format: "prmtop";
        is_binary: false;
    };
    top: {
        format: "top";
        is_binary: false;
    };
    map: {
        format: "map";
        is_binary: true;
    };
    dx: {
        format: "dx";
        is_binary: false;
    };
    dxbin: {
        format: "dxbin";
        is_binary: true;
    };
};
/** Convert MolViewSpec tree into MolStar tree */
export declare function convertMvsToMolstar(mvsTree: MVSTree, sourceUrl: string | undefined): MolstarTree;
/** Run some sanity check on a MVSTree. Return a list of potential problems (`undefined` if there are none) */
export declare function mvsSanityCheckIssues(tree: MVSTree): string[] | undefined;
/** Run some sanity check on a MVSTree and print potential issues to the console. */
export declare function mvsSanityCheck(tree: MVSTree): void;
