/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author Adam Midlik <midlik@gmail.com>
 */
import { SortedArray } from '../../../mol-data/int.js';
import { ChainIndex, ElementIndex, Model, ResidueIndex } from '../../../mol-model/structure.js';
import { Mapping } from './utils.js';
export interface IndicesAndSortings {
    atomic?: AtomicIndicesAndSortings;
    spheres?: CoarseIndicesAndSortings;
    gaussians?: CoarseIndicesAndSortings;
}
export declare const IndicesAndSortings: {
    /** Get `IndicesAndSortings` for a model (use a cached value or create if not available yet) */
    get(model: Model): IndicesAndSortings;
    /** Create `IndicesAndSortings` for a model */
    create(model: Model): IndicesAndSortings;
};
/** Auxiliary data structure for efficiently finding chains/residues/atoms in an atomic model by their properties */
export interface AtomicIndicesAndSortings {
    chainsByLabelEntityId: Mapping<string, readonly ChainIndex[]>;
    chainsByLabelAsymId: Mapping<string, readonly ChainIndex[]>;
    chainsByAuthAsymId: Mapping<string, readonly ChainIndex[]>;
    residuesSortedByLabelSeqId: Mapping<ChainIndex, Sorting<ResidueIndex, number>>;
    residuesSortedByAuthSeqId: Mapping<ChainIndex, Sorting<ResidueIndex, number>>;
    residuesSortedBySourceIndex: Mapping<ChainIndex, Sorting<ResidueIndex, number>>;
    residuesByInsCode: Mapping<ChainIndex, Mapping<string, readonly ResidueIndex[]>>;
    residuesByLabelCompId: Mapping<ChainIndex, Mapping<string, readonly ResidueIndex[]>>;
    /** Indicates if each residue is listed only once in `residuesByLabelCompId` (i.e. if each residue has only one label_comp_id) */
    residuesByLabelCompIdIsPure: boolean;
    residuesByAuthCompId: Mapping<ChainIndex, Mapping<string, readonly ResidueIndex[]>>;
    /** Indicates if each residue is listed only once in `residuesByAuthCompId` (i.e. if each residue has only one auth_comp_id) */
    residuesByAuthCompIdIsPure: boolean;
    atomsById: Mapping<number, ElementIndex>;
    atomsBySourceIndex: Mapping<number, ElementIndex>;
}
/** Auxiliary data structure for efficiently finding chains/elements in a coarse model by their properties */
export interface CoarseIndicesAndSortings {
    /** Coarse equivalent to `model.atomicHierarchy.chains` */
    chains: {
        /** Number of chains */
        count: number;
        /** Maps chain index to `label_entity_id` value */
        label_entity_id: string[];
        /** Maps chain index to `label_asym_id` value */
        label_asym_id: string[];
    };
    chainsByEntityId: Mapping<string, readonly ChainIndex[]>;
    chainsByAsymId: Mapping<string, readonly ChainIndex[]>;
    /** Coarse elements (per chain) sorted by `seq_id_begin`.
     * This is used to get the range of elements which may overlap with a certain seq_id interval.
     *
     * (Filtering coarse elements by seq_id range is an interval search problem, so the worst-case-efficient solution would be to use a data structure optimized for that.
     * But that would be overkill if we expect that in most cases the coarse elements cover non-overlapping seq_id ranges.
     * So the current solution should be sufficient (fast for non-overlapping elements, while still correct if there are overlaps).) */
    elementsSortedBySeqIdBegin: Mapping<ChainIndex, Sorting<ElementIndex, number> & {
        /** Non-decreasing upper bound for `seq_id_end` values of elements as listed in `keys` (`seq_id_end.value(keys[i]) <= endUpperBounds[i]`) */
        endUpperBounds: SortedArray;
    }>;
}
/** Represents a set of things (keys) of type `K`, sorted by some property (value) of type `V` */
export interface Sorting<K, V extends number> {
    /** Keys sorted by their corresponding values */
    keys: readonly K[];
    /** Sorted values corresponding to each key (value for `keys[i]` is `values[i]`) */
    values: SortedArray<V>;
}
export declare const Sorting: {
    /** Create a `Sorting` from an array of keys and a function returning their corresponding values.
     * If two keys have the same value, the smaller key will come first.
     * This function modifies `keys` - create a copy if you need the original order! */
    create<K extends number, V extends number>(keys: K[], valueFunction: (k: K) => V): Sorting<K, V>;
    /** Return a newly allocated array of keys which have value equal to `target`.
     * The returned keys are sorted by their value. */
    getKeysWithValue<K, V extends number>(sorting: Sorting<K, V>, target: V): K[];
    /** Return a newly allocated array of keys which have value within interval `[min, max]` (inclusive).
     * The returned keys are sorted by their value.
     * Undefined `min` is interpreted as negative infitity, undefined `max` is interpreted as positive infinity. */
    getKeysWithValueInRange<K, V extends number>(sorting: Sorting<K, V>, min: V | undefined, max: V | undefined): K[];
};
