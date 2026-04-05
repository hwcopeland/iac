/**
 * Copyright (c) 2017 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Structure } from '../../structure.js';
import { StructureSelection } from '../selection.js';
import { ElementIndex } from '../../model.js';
export declare class UniqueStructuresBuilder {
    private source;
    private set;
    private structures;
    private allSingletons;
    add(s: Structure): void;
    getSelection(): StructureSelection.Singletons | StructureSelection.Sequence;
    constructor(source: Structure);
}
export declare class LinearGroupingBuilder {
    private source;
    private builders;
    private builderMap;
    add(key: any, unit: number, element: ElementIndex): void;
    private allSingletons;
    private singletonSelection;
    private fullSelection;
    getSelection(): StructureSelection;
    constructor(source: Structure);
}
