/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Mat4 } from '../../mol-math/linear-algebra.js';
import { PluginContext } from '../../mol-plugin/context.js';
export type SuperpositionTestInput = {
    pdbId: string;
    auth_asym_id: string;
    matrix: Mat4;
}[];
export declare function buildStaticSuperposition(plugin: PluginContext, src: SuperpositionTestInput): Promise<void>;
export declare const StaticSuperpositionTestData: SuperpositionTestInput;
export declare function dynamicSuperpositionTest(plugin: PluginContext, src: string[], comp_id: string): Promise<void>;
export interface TMAlignResult {
    tmScoreA: number;
    tmScoreB: number;
    rmsd: number;
    alignedLength: number;
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
export declare function tmAlignStructures(plugin: PluginContext, pdbId1: string, chain1: string, pdbId2: string, chain2: string, color1?: number, color2?: number): Promise<TMAlignResult | undefined>;
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
export declare function loadStructuresNoAlignment(plugin: PluginContext, pdbId1: string, chain1: string, pdbId2: string, chain2: string, color1?: number, color2?: number): Promise<void>;
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
export declare function sequenceAlignStructures(plugin: PluginContext, pdbId1: string, chain1: string, pdbId2: string, chain2: string, color1?: number, color2?: number): Promise<{
    rmsd: number;
}>;
