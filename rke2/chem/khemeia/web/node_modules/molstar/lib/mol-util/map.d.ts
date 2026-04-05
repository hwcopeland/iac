/**
 * Copyright (c) 2018-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Alexander Rose <alexander.rose@weirdbyte.de>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
export interface DefaultMap<K, V> extends Map<K, V> {
    /** Return the value for `key` when available or the default value otherwise. */
    getDefault: (key: K) => V;
}
/** A `Map` instance with a `getDefault` method added. */
export declare function DefaultMap<K, V>(valueCtor: () => V): DefaultMap<K, V>;
export declare function arrayMapAdd<K, V>(map: Map<K, V[]>, key: K, value: V): void;
export declare class HashMap<K, V> {
    private hashCode;
    private areEqual;
    private buckets;
    set(key: K, value: V): void;
    get(key: K): V | undefined;
    delete(key: K): boolean;
    forEach(cb: (value: V, key: K) => void): void;
    clear(): void;
    constructor(hashCode: (key: K) => number, areEqual: (a: K, b: K) => boolean);
}
