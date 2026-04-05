/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
export interface PickingId {
    objectId: number;
    instanceId: number;
    groupId: number;
}
export declare namespace PickingId {
    const Null: 16777214;
    function areSame(a: PickingId, b: PickingId): boolean;
}
