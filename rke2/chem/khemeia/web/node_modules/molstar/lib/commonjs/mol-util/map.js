"use strict";
/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.HashMap = void 0;
exports.DefaultMap = DefaultMap;
exports.arrayMapAdd = arrayMapAdd;
/** A `Map` instance with a `getDefault` method added. */
function DefaultMap(valueCtor) {
    const map = new Map();
    map.getDefault = (key) => {
        if (map.has(key))
            return map.get(key);
        const value = valueCtor();
        map.set(key, value);
        return value;
    };
    return map;
}
// TODO currently not working, see https://github.com/Microsoft/TypeScript/issues/10853
// /** A `Map` with a `getDefault` method added. */
// export class DefaultMap<K, V> extends Map<K, V> {
//     constructor(private valueCtor: () => V, entries?: ReadonlyArray<[K, V]>) {
//         super(entries)
//     }
//     /** Return the value for `key` when available or the default value otherwise. */
//     getDefault(key: K) {
//         if (this.has(key)) return this.get(key)!
//         const value = this.valueCtor()
//         this.set(key, value)
//         return value
//     }
// }
function arrayMapAdd(map, key, value) {
    const a = map.get(key);
    if (a) {
        a.push(value);
    }
    else {
        map.set(key, [value]);
    }
}
class HashMap {
    set(key, value) {
        const hashCode = this.hashCode(key);
        let bucket = this.buckets.get(hashCode);
        if (!bucket) {
            bucket = [];
            this.buckets.set(hashCode, bucket);
        }
        for (let i = 0; i < bucket.length; i++) {
            if (this.areEqual(bucket[i].key, key)) {
                bucket[i].value = value;
                return;
            }
        }
        bucket.push({ key, value });
    }
    get(key) {
        const hashCode = this.hashCode(key);
        const bucket = this.buckets.get(hashCode);
        if (!bucket) {
            return undefined;
        }
        for (const entry of bucket) {
            if (this.areEqual(entry.key, key)) {
                return entry.value;
            }
        }
        return undefined;
    }
    delete(key) {
        const hashCode = this.hashCode(key);
        const bucket = this.buckets.get(hashCode);
        if (!bucket) {
            return false;
        }
        for (let i = 0; i < bucket.length; i++) {
            if (this.areEqual(bucket[i].key, key)) {
                bucket.splice(i, 1);
                if (bucket.length === 0) {
                    this.buckets.delete(hashCode);
                }
                return true;
            }
        }
        return false;
    }
    forEach(cb) {
        for (const bucket of this.buckets.values()) {
            for (const entry of bucket) {
                cb(entry.value, entry.key);
            }
        }
    }
    clear() {
        this.buckets.clear();
    }
    constructor(hashCode, areEqual) {
        this.hashCode = hashCode;
        this.areEqual = areEqual;
        this.buckets = new Map();
    }
}
exports.HashMap = HashMap;
