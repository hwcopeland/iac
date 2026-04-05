/**
 * Copyright (c) 2023-2026 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 * @author David Sehnal <david.sehnal@gmail.com>
 */
import { hashString } from '../../../mol-data/util.js';
import { range } from '../../../mol-util/array.js';
import { Color } from '../../../mol-util/color/index.js';
import { decodeColor as _decodeColor } from '../../../mol-util/color/utils.js';
/** Try to await a promise and return an object with its result (if resolved) or with the error (if rejected) */
export async function safePromise(promise) {
    try {
        const value = await promise;
        return { ok: true, value };
    }
    catch (error) {
        return { ok: false, error };
    }
}
/** A map where values are arrays. Handles missing keys when adding values. */
export class MultiMap {
    constructor() {
        this._map = new Map();
    }
    /** Return the array of values assidned to a key (or `undefined` if no such values) */
    get(key) {
        return this._map.get(key);
    }
    /** Append value to a key (handles missing keys) */
    add(key, value) {
        if (!this._map.has(key)) {
            this._map.set(key, []);
        }
        this._map.get(key).push(value);
    }
}
/** Implementation of `Map` where keys are integers
 * and most keys are expected to be from interval `[0, limit)`.
 * For the keys within this interval, performance is better than `Map` (implemented by array).
 * For the keys out of this interval, performance is slightly worse than `Map`. */
export class NumberMap {
    constructor(limit) {
        this.limit = limit;
        this.array = new Array(limit);
        this.map = new Map();
    }
    get(key) {
        if (0 <= key && key < this.limit)
            return this.array[key];
        else
            return this.map.get(key);
    }
    set(key, value) {
        if (0 <= key && key < this.limit)
            this.array[key] = value;
        else
            this.map.set(key, value);
    }
}
/** Return `true` if `value` is not `undefined` or `null`.
 * Prefer this over `value !== undefined`
 * (for maybe if we want to allow `null` in `AnnotationRow` in the future) */
export function isDefined(value) {
    return value !== undefined && value !== null;
}
/** Return `true` if at least one of `values` is not `undefined` or `null`. */
export function isAnyDefined(...values) {
    return values.some(isDefined);
}
/** Return filtered array containing all original elements except `undefined` or `null`. */
export function filterDefined(elements) {
    return elements.filter(x => x !== undefined && x !== null);
}
/** Create an 8-hex-character hash for a given input string, e.g. 'spanish inquisition' -> '7f9ac4be' */
function stringHash32(input) {
    const uint32hash = hashString(input) >>> 0; // >>>0 converts to uint32, LOL
    return uint32hash.toString(16).padStart(8, '0');
}
/** Create an 16-hex-character hash for a given input string, e.g. 'spanish inquisition' -> '7f9ac4be544330be'*/
export function stringHash(input) {
    const reversed = input.split('').reverse().join('');
    return stringHash32(input) + stringHash32(reversed);
}
/** Convert `colorString` (either X11 color name like 'magenta' or hex code like '#ff00ff') to Color.
 * Return `undefined` if `colorString` cannot be converted. */
export function decodeColor(colorString) {
    if (typeof colorString === 'number') {
        return Color(colorString);
    }
    return _decodeColor(colorString);
}
export function collectMVSReferences(type, dependencies) {
    const ret = {};
    for (const key of Object.keys(dependencies)) {
        const o = dependencies[key];
        let okType = false;
        for (const t of type) {
            if (t.is(o)) {
                okType = true;
                break;
            }
        }
        if (!okType || !o.tags)
            continue;
        for (const tag of o.tags) {
            if (tag.startsWith('mvs-ref:')) {
                ret[tag.substring(8)] = o.data;
                break;
            }
        }
    }
    return ret;
}
export function getMVSReferenceObject(type, dependencies, ref) {
    if (!dependencies)
        return undefined;
    for (const key of Object.keys(dependencies)) {
        const o = dependencies[key];
        let okType = false;
        for (const t of type) {
            if (t.is(o)) {
                okType = true;
                break;
            }
        }
        if (!okType || !o.tags)
            continue;
        for (const tag of o.tags) {
            if (tag.startsWith('mvs-ref:')) {
                if (tag.substring(8) === ref)
                    return o;
            }
        }
    }
}
export const GroupedArray = {
    getGroup(groupedArray, iGroup) {
        return groupedArray.grouped.slice(groupedArray.offsets[iGroup], groupedArray.offsets[iGroup + 1]);
    },
    /** Return element indices grouped by `group_by(element, index)`. Elements with `group_by(element, index)===undefined` are treated as separate groups. */
    groupIndices(elements, group_by) {
        let counter = 0;
        const groupMap = new Map();
        const groups = [];
        for (let i = 0; i < elements.length; i++) {
            const groupId = group_by(elements[i], i);
            if (!isDefined(groupId)) {
                groups.push(counter++);
            }
            else {
                const groupIndex = groupMap.get(groupId);
                if (groupIndex === undefined) {
                    groupMap.set(groupId, counter);
                    groups.push(counter);
                    counter++;
                }
                else {
                    groups.push(groupIndex);
                }
            }
        }
        const elementIndices = range(elements.length).sort((i, j) => groups[i] - groups[j]);
        const offsets = [];
        for (let i = 0; i < elements.length; i++) {
            if (i === 0 || groups[elementIndices[i]] !== groups[elementIndices[i - 1]])
                offsets.push(i);
        }
        offsets.push(elementIndices.length);
        return { count: offsets.length - 1, offsets, grouped: elementIndices };
    },
};
