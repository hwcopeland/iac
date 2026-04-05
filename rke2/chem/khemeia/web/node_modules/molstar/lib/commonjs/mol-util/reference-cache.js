"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.createReference = createReference;
exports.createReferenceItem = createReferenceItem;
exports.createReferenceCache = createReferenceCache;
const array_1 = require("./array.js");
function createReference(value, usageCount = 0) {
    return { value, usageCount };
}
function createReferenceItem(ref) {
    return {
        free: () => {
            ref.usageCount -= 1;
        },
        value: ref.value
    };
}
function createReferenceCache(hashFn, ctor, deleteFn) {
    const map = new Map();
    const values = [];
    return {
        get: (props) => {
            const id = hashFn(props);
            let ref = map.get(id);
            if (!ref) {
                ref = createReference(ctor(props));
                map.set(id, ref);
                values.push(ref.value);
            }
            ref.usageCount += 1;
            return createReferenceItem(ref);
        },
        clear: () => {
            map.forEach((ref, id) => {
                if (ref.usageCount <= 0) {
                    if (ref.usageCount < 0) {
                        console.warn('Reference usageCount below zero.');
                    }
                    deleteFn(ref.value);
                    map.delete(id);
                    (0, array_1.arrayRemoveInPlace)(values, ref.value);
                }
            });
        },
        get count() {
            return map.size;
        },
        values,
        dispose: () => {
            map.forEach(ref => deleteFn(ref.value));
            map.clear();
            values.length = 0;
        },
    };
}
