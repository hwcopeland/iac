/**
 * Copyright (c) 2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { Structure } from '../../mol-model/structure.js';
import { InteractionElementSchema, StructureInteractions } from './model.js';
export declare function getCustomInteractionData(interactions: InteractionElementSchema[], structures: {
    [ref: string]: Structure;
}): StructureInteractions;
