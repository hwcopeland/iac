/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { InteractionsProps } from '../../mol-model-props/computed/interactions.js';
import { StructureElement } from '../../mol-model/structure.js';
import { RuntimeContext } from '../../mol-task/index.js';
import { StructureInteractions } from './model.js';
export interface ComputeInteractionsOptions {
    interactions?: InteractionsProps;
}
export declare function computeContacts(ctx: RuntimeContext, selection: readonly {
    structureRef: string;
    loci: StructureElement.Loci;
}[], options?: ComputeInteractionsOptions): Promise<StructureInteractions>;
