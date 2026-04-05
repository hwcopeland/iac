/**
 * Copyright (c) 2017-2024 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Structure } from '../structure.js';
import { StructureSelection } from './selection.js';
import { QueryFn, QueryContextOptions } from './context.js';
interface StructureQuery extends QueryFn<StructureSelection> {
}
declare namespace StructureQuery {
    function run(query: StructureQuery, structure: Structure, options?: QueryContextOptions): StructureSelection;
    function loci(query: StructureQuery, structure: Structure, options?: QueryContextOptions): import("../structure/element/loci.js").Loci;
}
export { StructureQuery };
