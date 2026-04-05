/**
 * Copyright (c) 2017 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Structure } from '../structure.js';
export declare class StructureUniqueSubsetBuilder {
    private parent;
    private ids;
    private unitMap;
    private parentId;
    private currentUnit;
    elementCount: number;
    addToUnit(parentId: number, e: number): void;
    has(parentId: number, e: number): boolean;
    beginUnit(parentId: number): void;
    addElement(e: number): void;
    commitUnit(): void;
    getStructure(): Structure;
    get isEmpty(): boolean;
    constructor(parent: Structure);
}
