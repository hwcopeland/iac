/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
export var PickingId;
(function (PickingId) {
    PickingId.Null = 16777214; // Math.pow(2, 24) - 2
    function areSame(a, b) {
        return a.objectId === b.objectId && a.instanceId === b.instanceId && a.groupId === b.groupId;
    }
    PickingId.areSame = areSame;
})(PickingId || (PickingId = {}));
