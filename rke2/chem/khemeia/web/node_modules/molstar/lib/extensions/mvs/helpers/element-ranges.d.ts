/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { SortedArray } from '../../../mol-data/int.js';
import { ElementIndex } from '../../../mol-model/structure.js';
/** Represents a collection of disjoint elements ranges in a model (atoms, spheres, or gaussians).
 * The number of ranges is `ElementRanges.count(ranges)`,
 * the i-th range covers elements `[ranges.from[i], ranges.to[i])`. */
export interface ElementRanges {
    from: ElementIndex[];
    to: ElementIndex[];
}
export declare const ElementRanges: {
    /** Return the number of disjoined ranges in a `ElementRanges` object */
    count(ranges: ElementRanges): number;
    /** Create new `ElementRanges` without any elements */
    empty(): ElementRanges;
    /** Create new `ElementRanges` containing a single range of elements `[from, to)` */
    single(from: ElementIndex, to: ElementIndex): ElementRanges;
    /** Add a range of elements `[from, to)` to existing `ElementRanges` and return the modified original.
     * The added range must start after the end of the last existing range
     * (if it starts just on the next element, these two ranges will get merged). */
    add(ranges: ElementRanges, from: ElementIndex, to: ElementIndex): ElementRanges;
    /** Apply function `func` to each range in `ranges` */
    foreach(ranges: ElementRanges, func: (from: ElementIndex, to: ElementIndex) => any): void;
    /** Apply function `func` to each range in `ranges` and return an array with results */
    map<T>(ranges: ElementRanges, func: (from: ElementIndex, to: ElementIndex) => T): T[];
    /** Compute the set union of multiple `ElementRanges` objects (as sets of elements) */
    union(ranges: (ElementRanges | undefined)[]): ElementRanges;
    /** Return a sorted subset of `elements` which lie in any of `ranges` (i.e. set intersection of `elements` and `ranges`).
     * If `out` is provided, use it to store the result (clear any old contents).
     * If `outFirstElementIndex` is provided, fill `outFirstElementIndex.value` with the index of the first selected element (if any). */
    selectElementsInRanges(elements: SortedArray<ElementIndex>, ranges: ElementRanges, out?: ElementIndex[], outFirstElementIndex?: {
        value?: number;
    }): ElementIndex[];
};
