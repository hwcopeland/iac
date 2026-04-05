/**
 * Copyright (c) 2019 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
import { Expression } from './language/expression.js';
import { StructureElement, StructureSelection, Structure, QueryFn, QueryContextOptions } from '../mol-model/structure.js';
import { MolScriptBuilder } from './language/builder.js';
import { Script } from './types.js';
export { ScriptImpl as Script };
type ScriptImpl = Script;
declare function ScriptImpl(expression: string, language: Script['language']): Script;
declare namespace ScriptImpl {
    const Info: {
        [k in Script['language']]: string;
    };
    type Language = Script['language'];
    function is(x: any): x is Script;
    function areEqual(a: Script, b: Script): boolean;
    function toExpression(script: Script): Expression;
    function toQuery(script: Script): QueryFn<StructureSelection>;
    function toLoci(script: Script, structure: Structure): StructureElement.Loci;
    function getStructureSelection(expr: Expression | ((builder: typeof MolScriptBuilder) => Expression), structure: Structure, options?: QueryContextOptions): StructureSelection;
}
