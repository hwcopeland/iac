/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { SortedArray } from '../../../../mol-data/int.js';
import { Structure } from '../structure.js';
import { SortedRanges } from '../../../../mol-data/int/sorted-ranges.js';
import { UnitIndex } from './element.js';
import { Loci } from './loci.js';
import { Expression } from '../../../../mol-script/language/expression.js';
import { MolScriptBuilder as MS } from '../../../../mol-script/language/builder.js';
import { QueryContext, QueryFn, StructureSelection } from '../../query.js';
import { Schema } from './schema.js';
export interface BundleElement {
    /**
     * Array (sorted by first element in sub-array) of
     * arrays of `Unit.id`s that share the same `Unit.invariantId`
     */
    groupedUnits: SortedArray<number>[];
    set: SortedArray<UnitIndex>;
    ranges: SortedRanges<UnitIndex>;
}
export interface Bundle {
    /** Hash of the structure with which the bundle is compatible */
    readonly hash: number;
    /** Bundle elements */
    readonly elements: ReadonlyArray<Readonly<BundleElement>>;
}
export declare namespace Bundle {
    const Empty: Bundle;
    function fromSubStructure(parent: Structure, structure: Structure): Bundle;
    function fromSelection(selection: StructureSelection): Bundle;
    function fromExpression(structure: Structure, expression: Expression | ((builder: typeof MS) => Expression), queryContext?: QueryContext): Bundle;
    function fromQuery(structure: Structure, query: QueryFn, queryContext?: QueryContext): Bundle;
    function fromSchema(structure: Structure, schema: Schema, queryContext?: QueryContext): Bundle;
    function fromLoci(loci: Loci): Bundle;
    function toLoci(bundle: Bundle, structure: Structure): Loci;
    function toStructure(bundle: Bundle, parent: Structure): Structure;
    function toExpression(bundle: Bundle): Expression;
    function areEqual(a: Bundle, b: Bundle): boolean;
}
