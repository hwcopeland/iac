/**
 * Copyright (c) 2020 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { ParamDefinition as PD } from '../../mol-util/param-definition.js';
import { Expression } from '../../mol-script/language/expression.js';
import { StructureElement, Structure } from '../../mol-model/structure.js';
import { StructureQueryHelper } from './structure-query.js';
import { PluginStateObject as SO } from '../objects.js';
import { StateTransformer, StateObject } from '../../mol-state/index.js';
export declare const StaticStructureComponentTypes: readonly ["all", "polymer", "protein", "nucleic", "water", "ion", "lipid", "branched", "ligand", "non-standard", "coarse"];
export type StaticStructureComponentType = (typeof StaticStructureComponentTypes)[number];
export declare const StructureComponentParams: () => {
    type: PD.Mapped<PD.NamedParams<"all" | "polymer" | "water" | "branched" | "ligand" | "ion" | "lipid" | "protein" | "coarse" | "nucleic" | "non-standard", "static"> | PD.NamedParams<import("../../mol-script/types.js").Script, "script"> | PD.NamedParams<Expression, "expression"> | PD.NamedParams<StructureElement.Bundle, "bundle">>;
    nullIfEmpty: PD.Base<boolean | undefined>;
    label: PD.Text<string>;
};
export type StructureComponentParams = PD.ValuesFor<ReturnType<typeof StructureComponentParams>>;
export declare function createStructureComponent(a: Structure, params: StructureComponentParams, cache: {
    source: Structure;
    entry?: StructureQueryHelper.CacheEntry;
}): StateObject<any, any> | SO.Molecule.Structure;
export declare function updateStructureComponent(a: Structure, b: SO.Molecule.Structure, oldParams: StructureComponentParams, newParams: StructureComponentParams, cache: {
    source: Structure;
    entry?: StructureQueryHelper.CacheEntry;
}): StateTransformer.UpdateResult;
