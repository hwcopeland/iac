"use strict";
/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ElementRanges = void 0;
const int_1 = require("../../../mol-data/int.js");
const array_1 = require("../../../mol-util/array.js");
exports.ElementRanges = {
    /** Return the number of disjoined ranges in a `ElementRanges` object */
    count(ranges) {
        return ranges.from.length;
    },
    /** Create new `ElementRanges` without any elements */
    empty() {
        return { from: [], to: [] };
    },
    /** Create new `ElementRanges` containing a single range of elements `[from, to)` */
    single(from, to) {
        return { from: [from], to: [to] };
    },
    /** Add a range of elements `[from, to)` to existing `ElementRanges` and return the modified original.
     * The added range must start after the end of the last existing range
     * (if it starts just on the next element, these two ranges will get merged). */
    add(ranges, from, to) {
        const n = exports.ElementRanges.count(ranges);
        if (n > 0) {
            const lastTo = ranges.to[n - 1];
            if (from < lastTo)
                throw new Error('Overlapping ranges not allowed');
            if (from === lastTo) {
                ranges.to[n - 1] = to;
            }
            else {
                ranges.from.push(from);
                ranges.to.push(to);
            }
        }
        else {
            ranges.from.push(from);
            ranges.to.push(to);
        }
        return ranges;
    },
    /** Apply function `func` to each range in `ranges` */
    foreach(ranges, func) {
        const n = exports.ElementRanges.count(ranges);
        for (let i = 0; i < n; i++)
            func(ranges.from[i], ranges.to[i]);
    },
    /** Apply function `func` to each range in `ranges` and return an array with results */
    map(ranges, func) {
        const n = exports.ElementRanges.count(ranges);
        const result = new Array(n);
        for (let i = 0; i < n; i++)
            result[i] = func(ranges.from[i], ranges.to[i]);
        return result;
    },
    /** Compute the set union of multiple `ElementRanges` objects (as sets of elements) */
    union(ranges) {
        const concat = exports.ElementRanges.empty();
        for (const r of ranges) {
            if (r) {
                (0, array_1.arrayExtend)(concat.from, r.from);
                (0, array_1.arrayExtend)(concat.to, r.to);
            }
        }
        const indices = (0, array_1.range)(concat.from.length).sort((i, j) => concat.from[i] - concat.from[j]); // sort by start of range
        const result = exports.ElementRanges.empty();
        let last = -1;
        for (const i of indices) {
            const from = concat.from[i];
            const to = concat.to[i];
            if (last >= 0 && from <= result.to[last]) {
                if (to > result.to[last]) {
                    result.to[last] = to;
                }
            }
            else {
                result.from.push(from);
                result.to.push(to);
                last++;
            }
        }
        return result;
    },
    /** Return a sorted subset of `elements` which lie in any of `ranges` (i.e. set intersection of `elements` and `ranges`).
     * If `out` is provided, use it to store the result (clear any old contents).
     * If `outFirstElementIndex` is provided, fill `outFirstElementIndex.value` with the index of the first selected element (if any). */
    selectElementsInRanges(elements, ranges, out, outFirstElementIndex = {}) {
        var _a, _b;
        out !== null && out !== void 0 ? out : (out = []);
        out.length = 0;
        outFirstElementIndex.value = undefined;
        const nElements = elements.length;
        const nRanges = exports.ElementRanges.count(ranges);
        if (nElements <= nRanges) {
            // Implementation 1 (more efficient when there are fewer elements)
            let iRange = int_1.SortedArray.findPredecessorIndex(int_1.SortedArray.ofSortedArray(ranges.to), elements[0] + 1);
            for (let iElem = 0; iElem < nElements; iElem++) {
                const a = elements[iElem];
                while (iRange < nRanges && ranges.to[iRange] <= a)
                    iRange++;
                const qualifies = iRange < nRanges && ranges.from[iRange] <= a;
                if (qualifies) {
                    out.push(a);
                    (_a = outFirstElementIndex.value) !== null && _a !== void 0 ? _a : (outFirstElementIndex.value = iElem);
                }
            }
        }
        else {
            // Implementation 2 (more efficient when there are fewer ranges)
            for (let iRange = 0; iRange < nRanges; iRange++) {
                const from = ranges.from[iRange];
                const to = ranges.to[iRange];
                for (let iElem = int_1.SortedArray.findPredecessorIndex(elements, from); iElem < nElements; iElem++) {
                    const a = elements[iElem];
                    if (a < to) {
                        out.push(a);
                        (_b = outFirstElementIndex.value) !== null && _b !== void 0 ? _b : (outFirstElementIndex.value = iElem);
                    }
                    else {
                        break;
                    }
                }
            }
        }
        return out;
    },
};
